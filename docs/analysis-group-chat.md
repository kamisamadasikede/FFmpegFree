# Agent 群聊功能分析

日期：2026-10-21　分支：`v2`（群聊协调器第一期**未提交**）

截图：[派活](analysis-group/g-dispatch.png) · [全员介绍](analysis-group/g-coord.png) · [多人并行](analysis-group/g-working.png) · [主流程](analysis-group/g-main.png)

---

## 一、整体判断

**骨架很强，链路断在"为什么是他"。**

群聊不是 demo 级功能。它有完整的数据模型（request / assignment / workflow / message 四层）、持久化派发 outbox、启动恢复、能力上限（chat/read_only/write）、改派与重试的围栏机制、DAG 依赖与验收返工。这套东西的完成度超过项目里绝大多数模块。

但用户实际看到的是一个**黑箱**：你发一句话，某个成员开始回，中间什么都不显示。协调器明明判断了"该谁接、为什么"，这条信息在后端生成了、落库了、也发到前端了 —— 然后被前端自己过滤掉。

这是当前最刺眼的问题，而且修复成本极低（删两行过滤 + 渲染已有的数据）。

| 维度 | 评价 |
|---|---|
| 数据模型 | **A** — 四层对象职责清晰，ChainID 只当兼容键 |
| 持久化与恢复 | **A-** — 事务边界正确，不盲目重放副作用 |
| 执行层 | **C+** — 有真 bug：任务会永久卡在"排队中" |
| 协调器 | **B** — 判断逻辑完整，但配置绑死第一名成员 |
| 前端呈现 | **D** — 核心信息被过滤，多个状态切换错误 |
| 文档 | **B** — 设计文档诚实（明确写了未完成项），但契约未同步 |

---

## 二、架构：三层派发

```
用户发消息
  └─ SendCatGroupMessage (group_route.go:26)
       ├─ 保存消息 + request + 接收快照   ← 同事务，立即返回
       └─ 异步 launchGroupRequest
            └─ coordinateGroupRequest (group_coordinator.go:97)
                 ├─ ClaimCatGroupRequest        ← 条件认领，防重复
                 ├─ decideRoute → runAIRoute    ← 2 分钟超时的模型调用
                 │    └─ 输出 JSON: {memberIds, reason, everyone, tasks}
                 ├─ parseGroupWorkPlan          ← ≤12 任务、环检测、恰好一个 verify
                 └─ CommitCatGroupDecision      ← 单事务：request=assigned + 全部 assignment + workflow
                      └─ wakeMember（逐个，hop=0）
```

**做得好的地方**（这些不要动）：

- **接收与执行解耦**。发送接口落库后立即返回，不等待模型。用户输入不卡。
- **条件认领**。`ClaimCatGroupRequest` 用状态 CAS 认领，重复唤醒不会重复启动同一派发。
- **固定输入快照**。`assignmentSnapshot`（group_coordinator.go:222）只用接收时的快照 + 本任务依赖结果，**不读成员未读游标**。这避免了"连续发消息时新消息混进旧任务"这个多 Agent 系统最常见的 bug。
- **迟到结果围栏**。`ErrCatGroupAssignmentInactive`（group_turn.go:548）+ `status='running'` CAS（store/cat_group_requests.go:315）能把停止/改派后迟到的结果挡在群外。这块设计是对的。
- **drainGroupQueue 的 owner 代际比较**（group_turn.go:606）—— 旧世代不会 drain 新队列。

---

## 三、P0：任务会永久卡在"排队中"

**这是最严重的 bug，因为它是用户可见的死状态。**

DAG 节点被置为 `queued` 并落库后，如果唤醒失败，代码静默返回：

```go
// group_turn.go:116-119
if err != nil || !claimed {
    return          // ← 没有回滚 queued，没有重试，没有任何日志以外的东西
}
// group_turn.go:191-195  claim 失败
// group_turn.go:204-207  beginGroupWorkTask 失败
```

而**会话内没有任何 reconcile 重扫 `queued`**：
- `readyGroupTasks` 只取 `pending`（group_workflow.go:299-324）
- `retryGroupWorkTask` 只取 `blocked/failed/stopped`
- `recoverGroupWorkflows` 只在**启动时**跑一次

结果：该节点永久"排队中"，**整条依赖链永久等待**。用户看到的是一个永远不动的进度条，重启应用才能恢复。

修复方向：`queued` 需要一个带超时的看门狗，或把 wake 失败改成 `blocked` + 可见原因（后者更符合现有语义）。

---

## 四、P0：协调信息被前端自己过滤掉

后端在 `group_route.go:135-136` 写入 `systemKind="coord"` 的气泡，内容是 `formatCoordBubbleText(reason, everyone, members, memberIDs)` —— 即"派给谁 + 为什么"。

前端：

```js
// CatGroupView.vue:296-297
if (['coord', 'report_format', 'no_handoff'].includes(m.systemKind ?? '')) return false
if (m.systemKind === 'route') return m.route?.mode === 'undecided'
```

**成功路径的 `coord` 和 `route` 全被丢弃**，只保留 `undecided`（失败态）。所以：

- 用户消息 → 成员回复之间**什么都不显示**；
- `route.reason`（≤40 字理由）、`route.everyone`、`route.durationMs` 全部拿到但没用；
- `GetCatRouteDetail`（api/catGroupChat.ts:497）和 `store.routeDetail`（store:420）是**零调用方的死接口**；
- 配套 8 个 i18n 键（routeDetail/Input/Output/Judge/Cli/Took/Saw/Raw…）成了孤儿。

截图印证：三个场景里用户消息与成员回复之间都是空白。

**讽刺的是**：`catGroupChatMock.ts` 里有 `coord` 走查场景，`.check.ts` 里测了 `coordBodyText` / `isCoordMessage` —— 测试覆盖了一个用户永远看不到的界面。

---

## 五、P0：写任务被静默降级为只读

```go
// group_route.go:321
detail, _ = s.cfg.Store.InsertCatGroupRoute(...)   // 错误被丢弃

// group_coordinator.go:135
detail, _ := s.cfg.Store.GetCatGroupRoute(routeCtx, route.RouteSeq)
decision := s.materializeGroupDecision(routeCtx, groupDecision(r.Message, targets, nil, detail.OutputText))
```

插入失败 → `RouteSeq = 0` → 取回的 `detail` 为空 → `groupDecisionCapability` 从空 `OutputText` 解析不到 `capability` → 落到默认分支：

```go
// group_coordinator.go:64-67
if hasTasks { return "write" }
return "read_only"        // ← 写任务变成只读
```

用户让它改代码，它只分析不改，**且不告诉用户为什么**。

同类问题：`group_turn.go:250` 的 `_ =` 在 request 读取失败时会把已解析的附件覆盖为空，**静默丢图**。

---

## 六、P1：并发与锁

**队列模型**：`groupQueues map[string]*memberQueueState`（group_turn.go:42，包级全局），按**成员**分队列，`items` 是**无界 FIFO**，每成员并发 1。

**全局没有信号量**。真正的串行来自 `agentexec` 的工作区租约（runtime.go:118-122 + group_turn.go:826 `Scope{Workspace: t.usageCwd}`）—— 同一 project root 实际并发 = 1。

**后果**：UI 会同时显示多个成员"running"（截图里"2 人在干活"），但第二个其实在等租约。而且：

```go
// group_turn.go:345  st.running 在这里就设置
// runtime.go:118-122 之后才等租约
```

`elapsed` 从设置那一刻开始涨，**等待租约的时间被算成执行时间**。某个 CLI 卡住时，同项目全员无限挂起，且等待无超时（只有 ctx 取消）。

**锁内做 OS 进程枚举**（真缺陷）：

```
StopCatGroup (group_turn.go:732) 持 groupDispatchMu + groupStartMu (:738)
  └─ stopCatGroupMember → catagent.StopLiveConv (live.go:50)
       └─ live_windows.go:16-43  CreateToolhelp32Snapshot
                                 + 每个 node/grok OpenProcess 读命令行
                                 + killProcessTree
```

期间 `SendCatGroupMessage`（group_route.go:81 抢 `groupDispatchMu`）被阻塞 —— **停止时发送按钮会卡住数秒 × 成员数**。`ReassignCatGroupRoute` 走同一条链。

**锁序**：dispatch → start → work → queue，一致，没有反序。这点是对的。

---

## 七、P1：重试与超时名不副实

| 机制 | 实际行为 |
|---|---|
| 自动重试 | `maxAutoGroupRetries=3`，退避 2/4/8s（group_retry.go:12,64-68）✓ |
| 重试计数 | 存 ctx value，**纯内存，重启清零** |
| 请求级 watchdog | `watchGroupReply`（group_coordinator.go:260-280）2 分钟 AfterFunc，**只发一条灰字，不取消不重试** |
| watchdog 返回值 | `stop` 闭包被丢弃（:92, :170）→ **不可取消** |
| 真正的超时 | adapter 的 10 分钟 idle watchdog（idle_watch.go:14） |
| 群回合总时长 | **无上限** |

最坏情况：4 × 10 分钟 + 退避 ≈ 40 分钟无输出，UI 一直显示"处理中"。

而且 watchdog 文案说"执行超时会自动重试"，与实现矛盾 —— 它只是发一句话。

---

## 八、P1：6 跳防环基本失效

```go
// group_route.go:18
const maxGroupHops = 6
```

只在**非托管**链路的 @ 转交里检查（group_turn.go:576）。但 v0.57 起**每条用户消息都走 `CommitCatGroupDecision`**，托管回合在 group_turn.go:563-565 直接 `return`，**@ 不转交也不 +1**。

所以 6 跳上限在正常路径几乎不可达。而计数还是靠读最近 200 条消息重建（`chainHopFor` group_route.go:728-744），长链会归零绕过。

顺带：文案"已连续转交 6 次"与实际差一（实际只完成 5 次转交）。

---

## 九、P1：前端"切换群聊"路径的三个确定性错误

这三个不是竞态，是必现：

**1. 跨群 seq 串号**

`seq` 每群从 1 开始（`cat_group_requests.go:72` `MAX(seq) WHERE group_id`），但 `revDone` / `expanded` / `reverted` / `retried` / `revertedSeqs` 只按 seq 做键，且切群不重置。

→ A 群撤销过 seq=42，B 群的 seq=42 也显示"已撤销"。

**2. 路由指示器永久残留**

`open()` 清了 live / processBlocks / processLoading，**但没清 routing**（store:267-272），而 `onRoute` 丢弃非当前群事件（:116）。

→ 切群后底部永久挂着"正在安排谁来接…"。

**3. 重开群丢失"正在干活"条**

`open()` 清 `live`（:267），只有 mock 会恢复（:287-291）。真实后端 `GetCatGroup` 只回 status/queue，`applyQueue` 只写 memberStatus 不写 live。

→ 干活条消失、抽屉底栏显示"空闲"，要等下次工具事件（group_turn.go:387）才恢复。

---

## 十、产品面：做了什么 / 缺什么

**完整**：成员列表（CLI+模型+职责+拖拽排序+状态）、排队显示、换人、停止/全部停止、职责规范编辑（含生成骨架与失败原因）、群设置、建群/加成员、@ 提及（IME 安全）、查看过程抽屉、工作流卡片。

**半成品**：
- 路由指示器 —— 只有一个底部灰点；
- 重试 —— 乐观标记，无 busy 守卫；
- 文件页签 —— 无 loading 态；
- 工作流 —— 只是列表，`dependsOn` 从未渲染，**没有 DAG 边**；
- 附件 —— 只支持 png/jpeg/gif/webp，≤20 张 ≤15MB。

**缺失**：
- **协调气泡**（"派给谁 / 为什么"）；
- **派活详情**（route detail 抽屉）。

一个有意思的证据：群聊三语各 228 个 i18n 键，其中 **62 个（27%）从未被引用** —— 正好勾勒出没做完的功能面：coordinator、routeDetail 系列、undecided 原因码、空态分步引导、footer 耗时/Token。

i18n 本身是干净的：生产群聊代码**无硬编码中文**，三语齐全。这点比项目其他模块好。

---

## 十一、其他值得记的

**配置绑死第一名成员**：协调模型用的是 `members[0]` 的 AgentKind / ModelID / ThinkLevel（group_route.go:172, 213-222）。第一成员换了模型，协调器跟着换；第一成员是 Codex 时，协调判断也走 Codex。设计文档已列为"尚未解除"。

**群成员每轮触发桌面通知**：`finishLocked`（service.go:1218-1221）对 `hook == nil` 的回合无条件 `go notifyOrdinaryTurn`，而群回合的 convID 是 `group:gid:mid`，查不到会话 → 通知正文退回"新对话"（notify.go:93-107）。用户会收到一堆无意义通知。

**内存无上限**：
- `groupQueues` 在 reassign 裁剪后若空且非 active **不删除**（:177-187, :415-425）；
- `groupWorkContexts` 对 blocked 工作流**永不清除**；
- `groupQueues` 是**包级变量**，多 Service 实例共享；
- 前端 `messages` / `processBlocks` 无上限，1 秒 ticker 永不清理。

**两套并发写策略混用**：`SaveCatGroupWorkflow` 是无 revision CAS 的 blind upsert（cat_group_workflows.go:38-48），而 handoff 用 revision CAS（cat_group_requests.go:458-472）。目前靠 `groupWorkMu` 串行没丢更新，但任何绕过它的写入会**静默覆盖整份 DAG**。

**走查钩子进了生产**：`?cat_group_scene`（catPanels.ts:34）、`?cat_group_tab`（CatGroupMembers.vue:197）、mock 静态 import（api:9）。

**`blocked` 不在 advance 的早退名单里**（group_workflow.go:348）—— 阻塞后仍会继续推进。

**潜在自死锁**：`groupContext`（group_lifecycle.go:14-30）是不可重入 Mutex，却被已持 `groupDispatchMu` 的链路调用（wakeMember:102、advance:338），只靠 ctx 已带 lifetime 短路。唯一无 ctx 调用是 `finishGroupWorkTask:492 → advance`，若 `groupWorkContexts[chainID]` 已被删则会去抢 `groupDispatchMu`。

---

## 十二、建议顺序

**P0（用户直接踩到）**

1. 修 `queued` 死状态 —— wake 失败改 `blocked` + 可见原因，或加 reconcile 看门狗
2. 打开 coord / route 气泡 —— 删 CatGroupView.vue:296-297 的两行过滤，渲染已有的 `reason`
3. 修 `group_route.go:321` 和 `group_turn.go:250` 的 `_ =` —— 写任务静默降级为只读、静默丢图，都必须显式报错
4. 修"切群"三连：seq 键加 groupId 前缀、open 清 routing、真实后端恢复 live

**P1**

5. 锁内不杀进程 —— 停止/改派时先释放锁，进程清理异步做
6. 接上 route detail 抽屉（接口和 i18n 都是现成的）
7. watchdog 改成真的能取消 + 加群回合总时长上限
8. 修通知噪声（群回合跳过 notifyOrdinaryTurn）
9. 让"2 人在干活"诚实 —— 等待租约的成员显示"等待中"而非"处理中"

**P2**

10. 协调模型独立配置，解除第一成员依赖
11. 工作流画 DAG 边（`dependsOn` 数据是有的）
12. 内存上限与清理（groupQueues / groupWorkContexts / 前端 messages）
13. 走查钩子移出生产（dev-only）

---

## 十三、关于"未提交"

这 1225 行新增 / 478 行删除全部**未提交**，包含迁移 `0022`。风险：

- 迁移一旦被某个开发实例应用，回退代码会留下孤儿表；
- 设计文档 `group-conversation-coordinator.md` 本身写明"未提交 Git commit，也未替换用户正在运行的开发服务"；
- 契约 `contract.md` **完全没有** 0020/0021/0022 和协调器的章节（只到 0019）。

建议先按上面 P0 四项修完，跑一遍群聊定向回归（`go test ./internal/service/cat -run Group` 与 `./internal/store -run Group` 目前是通的），再提交。
