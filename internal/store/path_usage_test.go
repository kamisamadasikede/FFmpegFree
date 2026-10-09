package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestTargetPathUsage(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	dir := t.TempDir()
	src := filepath.Join(dir, "源.txt")
	outDone := filepath.Join(dir, "已完成.pdf")
	outRun := filepath.Join(dir, "转换中.pdf")
	inRun := filepath.Join(dir, "输入中.docx")
	copyP := filepath.Join(dir, "uploads", "副本.txt")
	free := filepath.Join(dir, "空闲.txt")

	key := pathKeyOf(src)
	srcRow, _, err := s.UpsertConvertSourceKind(ctx, src, key, SourceKindDoc, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InsertTask(ctx, Task{ID: "T1", Type: TypeDocConvert, Status: StatusSucceeded, InputPaths: []string{src}, OutputPath: outDone, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertTask(ctx, Task{ID: "T2", Type: TypeOfficePDF, Status: StatusRunning, InputPaths: []string{inRun}, OutputPath: outRun, CreatedAt: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO convert_copies (`+copyColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,1,0,?,?)`,
		"C1", "S", src, key, 1, 1, copyP, CopyReady, 1, 1, nil, 1, nil); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		p                 string
		converting, inUse bool
	}{
		{src, false, true},
		{outDone, false, true},
		{outRun, true, true},
		{inRun, true, false},
		{copyP, false, true},
		{free, false, false},
	}
	for _, c := range cases {
		u, err := s.TargetPathUsage(ctx, c.p)
		if err != nil {
			t.Fatal(err)
		}
		if u.Converting != c.converting || u.InUse != c.inUse {
			t.Errorf("%s: got %+v, want converting=%v inUse=%v", filepath.Base(c.p), u, c.converting, c.inUse)
		}
	}

	// 排除自己这一行 / 这条记录：自己的原文件、自己的输出不算被别的记录用着；正在转换照样算
	excepts := []struct {
		p, ownSrc, ownTask string
		converting, inUse  bool
	}{
		{src, srcRow.SourceID, "", false, false},
		{src, "", "T1", false, true},
		{outDone, "", "T1", false, false},
		{outDone, srcRow.SourceID, "", false, true},
		{outRun, "", "T2", true, false},
	}
	for _, c := range excepts {
		u, err := s.TargetPathUsageExcept(ctx, c.p, c.ownSrc, c.ownTask)
		if err != nil {
			t.Fatal(err)
		}
		if u.Converting != c.converting || u.InUse != c.inUse {
			t.Errorf("except %s (%s/%s): got %+v, want converting=%v inUse=%v", filepath.Base(c.p), c.ownSrc, c.ownTask, u, c.converting, c.inUse)
		}
	}
}
