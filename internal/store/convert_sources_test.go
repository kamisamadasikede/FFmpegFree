package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"FFmpegFree/internal/paths"
)

// openAtVersion 建一个只应用到 maxVersion 的数据库（模拟升级前的旧库），返回路径。
func openAtVersion(t *testing.T, maxVersion int, seed func(db *sql.DB)) string {
	t.Helper()
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+p)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if _, err := db.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.version <= maxVersion {
			if err := s.applyMigration(ctx, m); err != nil {
				t.Fatal(err)
			}
		}
	}
	seed(db)
	db.Close()
	return p
}

func TestMigration0005AndBackfill(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	a := filepath.Join(dir, "in", "Movie.MKV")
	b := filepath.Join(dir, "in", "other.mov")
	p := openAtVersion(t, 4, func(db *sql.DB) {
		ins := func(id, typ, inputs, out string, created int64) {
			if _, err := db.Exec(`INSERT INTO tasks (id, type, status, title, input_paths, output_path, params, created_at)
				VALUES (?,?,?,?,?,?,'{}',?)`, id, typ, "succeeded", "T-"+id, inputs, out, created); err != nil {
				t.Fatal(err)
			}
		}
		ins("t1", "convert", `["`+a+`"]`, filepath.Join(dir, "o", "Movie.mp4"), 100)
		ins("t2", "convert", `["`+a+`"]`, filepath.Join(dir, "o", "Movie(1).mp4"), 300)
		ins("t3", "convert", `["`+b+`"]`, "", 200)
		ins("t4", "convert", `not json`, "", 50) // 坏的 inputPaths
		ins("t5", "edit_export", `[]`, filepath.Join(dir, "o", "Cut.MP4"), 400)
	})
	s, err := Open(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, _ := s.SchemaVersion(ctx); v < 5 {
		t.Fatalf("迁移未应用: %d", v)
	}
	// 迁移不改旧数据：旧任务都在，新列是默认值
	tk, err := s.GetTask(ctx, "t1")
	if err != nil || tk.SourceID != "" || tk.HiddenInTaskCenter || tk.Result != nil {
		t.Fatalf("%+v %v", tk, err)
	}
	srcN, names, err := s.BackfillConvertSources(ctx)
	if err != nil || srcN != 4 || names != 3 {
		t.Fatalf("回填 sources=%d names=%d err=%v", srcN, names, err)
	}
	t1, _ := s.GetTask(ctx, "t1")
	t2, _ := s.GetTask(ctx, "t2")
	t3, _ := s.GetTask(ctx, "t3")
	t4, _ := s.GetTask(ctx, "t4")
	t5, _ := s.GetTask(ctx, "t5")
	if t1.SourceID == "" || t1.SourceID != t2.SourceID || t3.SourceID == "" || t3.SourceID == t1.SourceID || t4.SourceID == "" || t5.SourceID != "" {
		t.Fatalf("source_id: %q %q %q %q %q", t1.SourceID, t2.SourceID, t3.SourceID, t4.SourceID, t5.SourceID)
	}
	src, err := s.GetConvertSource(ctx, t1.SourceID)
	if err != nil || src.Name != "Movie.MKV" || src.AddedAt != 100 || src.LastActivityAt != 300 {
		t.Fatalf("同一路径的行：addedAt=最早、lastActivityAt=最晚: %+v %v", src, err)
	}
	bad, _ := s.GetConvertSource(ctx, t4.SourceID)
	if bad.Path != "" || bad.Name != "T-t4" {
		t.Fatalf("坏路径的行 path 为空、name 用 title: %+v", bad)
	}
	var key string
	s.DB().QueryRow(`SELECT output_name_key FROM tasks WHERE id='t5'`).Scan(&key)
	if key != "cut.mp4" {
		t.Fatalf("非转换任务也补 output_name_key: %q", key)
	}
	var nSrc int
	s.DB().QueryRow(`SELECT COUNT(*) FROM convert_sources`).Scan(&nSrc)
	if nSrc != 3 {
		t.Fatalf("应有 3 行: %d", nSrc)
	}

	// 重跑：幂等，不新建行、不改 source_id
	srcN, names, err = s.BackfillConvertSources(ctx)
	if err != nil || srcN != 0 || names != 0 {
		t.Fatalf("重跑应什么都不做: %d %d %v", srcN, names, err)
	}
	s.DB().QueryRow(`SELECT COUNT(*) FROM convert_sources`).Scan(&nSrc)
	if again, _ := s.GetTask(ctx, "t2"); nSrc != 3 || again.SourceID != t1.SourceID {
		t.Fatalf("重跑改了数据: %d %+v", nSrc, again)
	}
	// 关掉再开（再次启动）：迁移不重复、回填仍幂等
	s.Close()
	s2, err := Open(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if n, m, err := s2.BackfillConvertSources(ctx); err != nil || n != 0 || m != 0 {
		t.Fatalf("%d %d %v", n, m, err)
	}
	// 回填后按路径 Upsert 命中同一行
	np, nk, _ := paths.Normalize(a)
	got, existed, err := s2.UpsertConvertSource(ctx, np, nk, 999)
	if err != nil || !existed || got.SourceID != t1.SourceID || got.LastActivityAt != 999 {
		t.Fatalf("%+v %v %v", got, existed, err)
	}
}

func TestBackfillBatches(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	dir := t.TempDir()
	n := backfillBatch + 7
	for i := 0; i < n; i++ {
		in := filepath.Join(dir, "f", string(rune('a'+i%26))+".mp4")
		if _, err := s.DB().Exec(`INSERT INTO tasks (id, type, status, input_paths, output_path, created_at) VALUES (?,?,?,?,?,?)`,
			"id"+strconv.Itoa(i), "convert", "failed", `["`+in+`"]`, filepath.Join(dir, "o", "x"+strconv.Itoa(i)+".mp4"), int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	srcN, names, err := s.BackfillConvertSources(ctx)
	if err != nil || srcN != n || names != n {
		t.Fatalf("%d %d %v", srcN, names, err)
	}
	var rows, left int
	s.DB().QueryRow(`SELECT COUNT(*) FROM convert_sources`).Scan(&rows)
	s.DB().QueryRow(`SELECT COUNT(*) FROM tasks WHERE source_id IS NULL`).Scan(&left)
	if rows != 26 || left != 0 {
		t.Fatalf("rows=%d left=%d", rows, left)
	}
}

func TestListConvertSourcesSearch(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	dir := t.TempDir()
	mk := func(name string, at int64) ConvertSource {
		p, k, _ := paths.Normalize(filepath.Join(dir, name))
		src, _, err := s.UpsertConvertSource(ctx, p, k, at)
		if err != nil {
			t.Fatal(err)
		}
		return src
	}
	holiday := mk("Holiday.MOV", 10)
	work := mk("work.mkv", 20)
	mk("misc.avi", 30)
	// work 的一条转换记录输出叫 "Summer Holiday.mp4"；一条非转换任务也叫 holiday，不算
	ins := func(id, typ, src, out string) {
		tk := Task{ID: id, Type: TaskType(typ), Status: "succeeded", OutputPath: out, SourceID: src, CreatedAt: 1, Version: 1}
		if err := s.InsertTask(ctx, tk); err != nil {
			t.Fatal(err)
		}
	}
	ins("c1", "convert", work.SourceID, filepath.Join(dir, "o", "Summer Holiday.mp4"))
	ins("c2", "convert", work.SourceID, filepath.Join(dir, "o", "work.mp4"))
	ins("e1", "edit_export", "", filepath.Join(dir, "o", "holiday-cut.mp4"))

	all, total, err := s.ListConvertSources(ctx, "", "", 50, 0)
	if err != nil || total != 3 || all[0].Name != "misc.avi" || all[2].Name != "Holiday.MOV" {
		t.Fatalf("lastActivityAt 倒序: %+v %d %v", all, total, err)
	}
	got, total, err := s.ListConvertSources(ctx, "holiday", "", 50, 0)
	if err != nil || total != 2 || len(got) != 2 || got[0].SourceID != work.SourceID || got[1].SourceID != holiday.SourceID {
		t.Fatalf("源文件名或输出名命中: %+v %d %v", got, total, err)
	}
	ids, _ := s.MatchedSourceTaskIDs(ctx, work.SourceID, "holiday", 200)
	if len(ids) != 1 || ids[0] != "c1" {
		t.Fatalf("%v", ids)
	}
	if got, total, _ := s.ListConvertSources(ctx, "nothing", "", 50, 0); total != 0 || len(got) != 0 {
		t.Fatalf("%+v", got)
	}
	// 分页
	if got, total, _ := s.ListConvertSources(ctx, "", "", 1, 1); total != 3 || len(got) != 1 || got[0].SourceID != work.SourceID {
		t.Fatalf("%+v", got)
	}
	// 记录不受任务中心隐藏影响
	if _, err := s.HideFinishedTasks(ctx); err != nil {
		t.Fatal(err)
	}
	if page, _ := s.ListSourceTasks(ctx, work.SourceID, 20, 0); page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("%+v", page)
	}
}

func TestHideUnhideTasks(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	for i, st := range []string{"succeeded", "failed", "running", "canceled"} {
		if err := s.InsertTask(ctx, Task{ID: "h" + strconv.Itoa(i), Type: "convert", Status: TaskStatus(st), CreatedAt: int64(i), Version: 1}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.HideFinishedTasks(ctx)
	if err != nil || n != 3 {
		t.Fatalf("%d %v", n, err)
	}
	if n, _ := s.HideFinishedTasks(ctx); n != 0 {
		t.Fatalf("重复隐藏应为 0: %d", n)
	}
	h0, _ := s.GetTask(ctx, "h0")
	if !h0.HiddenInTaskCenter || h0.Version != 1 {
		t.Fatalf("隐藏不改 version: %+v", h0)
	}
	if p, _ := s.ListTasks(ctx, TaskFilter{}); p.Total != 1 {
		t.Fatalf("默认不列出隐藏的: %d", p.Total)
	}
	if p, _ := s.ListTasks(ctx, TaskFilter{IncludeHidden: true}); p.Total != 4 {
		t.Fatalf("includeHidden: %d", p.Total)
	}
	got, err := s.UnhideTasks(ctx, []string{"h0", "h2"})
	if err != nil || len(got) != 1 || got[0].ID != "h0" || got[0].HiddenInTaskCenter || got[0].Version != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if got, _ := s.UnhideTasks(ctx, []string{"h0"}); len(got) != 0 {
		t.Fatalf("幂等: %+v", got)
	}
}

// 契约 v0.23.1：ListSources 的 status 筛选（EXISTS 子查询），分页和排序不变。
func TestListConvertSourcesStatusFilter(t *testing.T) {
	ctx := context.Background()
	s, _ := openTemp(t)
	dir := t.TempDir()
	rows := map[string]ConvertSource{}
	// 行名 → 该行记录的状态；lastActivityAt 依次递增（后面的排在前面）
	plan := []struct {
		name     string
		statuses []string
	}{
		{"none.mp4", nil},
		{"queued.mp4", []string{"succeeded", "queued"}},
		{"running.mp4", []string{"running"}},
		{"failed.mp4", []string{"failed", "succeeded"}},
		{"interrupted.mp4", []string{"interrupted"}},
		{"canceled.mp4", []string{"canceled", "succeeded"}},
		{"both.mp4", []string{"running", "failed"}},
	}
	n := 0
	for i, pl := range plan {
		p, k, _ := paths.Normalize(filepath.Join(dir, pl.name))
		src, _, err := s.UpsertConvertSource(ctx, p, k, int64(100+i))
		if err != nil {
			t.Fatal(err)
		}
		rows[pl.name] = src
		for _, st := range pl.statuses {
			n++
			if err := s.InsertTask(ctx, Task{ID: "t" + strconv.Itoa(n), Type: "convert", Status: TaskStatus(st), SourceID: src.SourceID, CreatedAt: int64(n), Version: 1}); err != nil {
				t.Fatal(err)
			}
		}
	}
	// 非转换任务的状态不算
	if err := s.InsertTask(ctx, Task{ID: "x", Type: "edit_export", Status: "failed", SourceID: rows["none.mp4"].SourceID, CreatedAt: 1, Version: 1}); err != nil {
		t.Fatal(err)
	}
	names := func(list []ConvertSource) string {
		var out []string
		for _, r := range list {
			out = append(out, r.Name)
		}
		return strings.Join(out, ",")
	}
	all, total, err := s.ListConvertSources(ctx, "", SourceStatusAll, 50, 0)
	if err != nil || total != 7 || len(all) != 7 {
		t.Fatalf("%d %v", total, err)
	}
	act, total, _ := s.ListConvertSources(ctx, "", SourceStatusActive, 50, 0)
	if total != 3 || names(act) != "both.mp4,running.mp4,queued.mp4" {
		t.Fatalf("active: %d %s", total, names(act))
	}
	fail, total, _ := s.ListConvertSources(ctx, "", SourceStatusFailed, 50, 0)
	if total != 3 || names(fail) != "both.mp4,interrupted.mp4,failed.mp4" {
		t.Fatalf("failed（canceled 不算）: %d %s", total, names(fail))
	}
	// 分页：total 是筛选后的总数，顺序不变
	p1, total, _ := s.ListConvertSources(ctx, "", SourceStatusFailed, 2, 0)
	p2, total2, _ := s.ListConvertSources(ctx, "", SourceStatusFailed, 2, 2)
	if total != 3 || total2 != 3 || names(p1) != "both.mp4,interrupted.mp4" || names(p2) != "failed.mp4" {
		t.Fatalf("分页: %s | %s", names(p1), names(p2))
	}
	// 与关键字同时用：AND
	if got, total, _ := s.ListConvertSources(ctx, "fail", SourceStatusFailed, 50, 0); total != 1 || names(got) != "failed.mp4" {
		t.Fatalf("%s", names(got))
	}
	if _, _, err := s.ListConvertSources(ctx, "", "canceled", 50, 0); err == nil {
		t.Fatal("未知值应报错")
	}
	if !ValidSourceStatus("") || !ValidSourceStatus("active") || !ValidSourceStatus("failed") || ValidSourceStatus("canceled") || ValidSourceStatus("Active") {
		t.Fatal("ValidSourceStatus")
	}
	// 用到 (source_id, status) 索引
	var plan2 string
	rowsQ, err := s.DB().Query(`EXPLAIN QUERY PLAN SELECT 1 FROM tasks t WHERE t.source_id = 'a' AND t.type = 'convert' AND t.status IN ('failed','interrupted')`)
	if err != nil {
		t.Fatal(err)
	}
	for rowsQ.Next() {
		var id, parent, notused int
		var detail string
		rowsQ.Scan(&id, &parent, &notused, &detail)
		plan2 += detail + ";"
	}
	rowsQ.Close()
	if !strings.Contains(plan2, "idx_tasks_source") {
		t.Fatalf("应走 source_id 索引: %s", plan2)
	}
}
