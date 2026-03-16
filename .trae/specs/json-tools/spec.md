# JSON 处理功能 Spec

## Why
用户需要一个能够比较两个 JSON 文档差异的工具，以及一个能够格式化（美化）JSON 的工具。当 JSON 格式错误时，还需要准确定位错误位置以便用户修复。

## What Changes
- [ ] 添加 JSON 比对功能：比较两个 JSON 对象的差异，支持结构对比和值对比
- [ ] 添加 JSON 格式化功能：将压缩的 JSON 格式化为易读的形式
- [ ] 添加 JSON 语法错误检测：验证 JSON 语法有效性
- [ ] 添加错误位置报告：准确定位 JSON 语法错误的位置（行号、列号）
- [ ] 遵循用户规则：DTO 或 struct 必须声明注释

## Impact
- Affected specs: 工具集合
- Affected code:
  - backend/contollers/ (新增 JSON 控制器)
  - frontend/src/views/ (新增 JSON 处理页面)
  - frontend/src/router/index.ts

## ADDED Requirements
### Requirement: JSON 格式化功能
系统 SHALL 支持将 JSON 字符串格式化为易读的形式

#### Scenario: 成功格式化有效 JSON
- **WHEN** 用户输入有效的 JSON 字符串并点击格式化
- **THEN** 系统返回格式化的 JSON，保留缩进和换行

#### Scenario: 格式化无效 JSON
- **WHEN** 用户输入无效的 JSON 字符串并点击格式化
- **THEN** 系统返回错误信息，包含错误位置（行号、列号）

### Requirement: JSON 比对功能
系统 SHALL 支持比较两个 JSON 对象的差异

#### Scenario: 比对两个相同的 JSON
- **WHEN** 用户输入两个相同的 JSON 并点击比对
- **THEN** 系统显示"两个 JSON 完全相同"的提示

#### Scenario: 比对两个不同的 JSON
- **WHEN** 用户输入两个不同的 JSON 并点击比对
- **THEN** 系统显示差异列表，包括：
  - 新增的字段
  - 删除的字段
  - 值发生变化的字段

#### Scenario: 比对结构不同的 JSON
- **WHEN** 用户输入结构不同的 JSON（一个数组，一个对象）
- **THEN** 系统显示结构差异提示

### Requirement: JSON 语法错误定位
系统 SHALL 在 JSON 语法错误时提供精确的错误位置

#### Scenario: 检测缺失引号错误
- **WHEN** 用户输入 `{"name": abc}` 并验证
- **THEN** 系统提示在第 1 行第 10 列附近有语法错误，错误原因：缺少引号

#### Scenario: 检测多余逗号错误
- **WHEN** 用户输入 `{"name": "test",}` 并验证
- **THEN** 系统提示在第 1 行第 18 列附近有语法错误，错误原因：多余逗号

#### Scenario: 检测括号不匹配错误
- **WHEN** 用户输入 `{"data": [1, 2, 3]` 并验证
- **THEN** 系统提示在第 1 行第 20 列附近有语法错误，错误原因：括号不匹配

## MODIFIED Requirements
无

## REMOVED Requirements
无

## 工程规范要求
- 所有 DTO/Struct 必须包含注释声明
- 遵循现有的代码结构和风格
