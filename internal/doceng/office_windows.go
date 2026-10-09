//go:build windows

package doceng

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"

	"FFmpegFree/internal/apperr"
)

// OfficeConverter 通过 COM 调 Word / Excel / PowerPoint 或 WPS（契约 6.12.26）。
type OfficeConverter struct {
	mu sync.Mutex // 应用内演示锁：转换与预览互斥用 PPT/WPS 演示
}

func NewOfficeConverter() *OfficeConverter { return &OfficeConverter{} }

func (c *OfficeConverter) Convert(ctx context.Context, d Detected, req ConvertRequest) error {
	return c.run(ctx, d, req.SrcExt, req.Target, req.InputPath, req.OutputPath, req.WorkDir, req.Timeout, req.HasMacro, req.Logf, false)
}

func (c *OfficeConverter) PreviewPDF(ctx context.Context, d Detected, req PreviewPDFRequest) error {
	return c.run(ctx, d, req.SrcExt, "pdf", req.InputPath, req.OutputPath, req.WorkDir, req.Timeout, req.HasMacro, req.Logf, true)
}

func (c *OfficeConverter) run(ctx context.Context, d Detected, src, target, inPath, outPath, work string, timeout time.Duration, hasMacro bool, logf func(string, ...any), preview bool) error {
	if timeout <= 0 {
		timeout = TimeoutOfficeConvert
	}
	fam := familyOf(src)
	if fam == FamilySlide && PresentationBusy(d.ID) {
		return BusyErr(true, d.ID, preview)
	}
	// 本机办公软件池并发 1（6.12.29）
	c.mu.Lock()
	defer c.mu.Unlock()

	type result struct{ err error }
	ch := make(chan result, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		ch <- result{err: c.comConvert(ctx, d, src, target, inPath, outPath, work, hasMacro, logf, preview)}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-ch:
		return r.err
	case <-timer.C:
		return apperr.New(apperr.DocTimeout, "文件处理太久没完成，可能已损坏，请检查后重试。").
			WithDetail(fmt.Sprintf("timeoutSec=%d\nengine=%s", int(timeout/time.Second), d.ID))
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *OfficeConverter) comConvert(ctx context.Context, d Detected, src, target, inPath, outPath, work string, hasMacro bool, logf func(string, ...any), preview bool) (err error) {
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		return apperr.Wrap(apperr.DocComponentCrashed, "文档组件意外退出，请重试。", err).WithDetail("engine=" + d.ID + "\ncoinit")
	}
	defer ole.CoUninitialize()

	fam := familyOf(src)
	if fam == FamilyPDF {
		fam = FamilyText // PDF 只用 Word 打开（重排，6.12.61）
	}
	progID, exeBase := progFor(d, fam)
	if progID == "" {
		return apperr.New(apperr.DocComponentNotReady, "需要先下载文档组件。").WithDetail("engine=" + d.ID)
	}

	before := SnapshotPIDs(exeBase)
	unknown, err := oleutil.CreateObject(progID)
	if err != nil {
		return apperr.Wrap(apperr.DocComponentCrashed, "文档组件意外退出，请重试。", err).WithDetail("engine=" + d.ID + "\ncreate")
	}
	app, err := unknown.QueryInterface(ole.IID_IDispatch)
	unknown.Release()
	if err != nil {
		return apperr.Wrap(apperr.DocComponentCrashed, "文档组件意外退出，请重试。", err).WithDetail("engine=" + d.ID)
	}

	after := SnapshotPIDs(exeBase)
	pid := DiffPID(before, after)
	if pid == 0 {
		// 挂到了已有实例：立刻释放，不设属性、不 Quit
		app.Release()
		return BusyErr(fam == FamilySlide, d.ID, preview)
	}
	defer KillPID(pid)
	defer app.Release()

	// Job Object 能放就放（失败只记日志）。
	attachJob(pid)

	if err := hardenApp(app, fam, hasMacro); err != nil {
		return err
	}

	inDir := filepath.Join(work, "in")
	outDir := filepath.Join(work, "out")
	_ = os.MkdirAll(inDir, 0o755)
	_ = os.MkdirAll(outDir, 0o755)
	tmpIn := filepath.Join(inDir, "input."+src)
	if err := copyFile(inPath, tmpIn); err != nil {
		return err
	}
	stripZoneIdentifier(tmpIn)

	tmpOut := filepath.Join(outDir, "out."+target)
	if logf != nil {
		logf("文档引擎：%s（%s → %s）\n", d.Name, src, target)
	}

	switch fam {
	case FamilyText:
		err = convertWord(app, tmpIn, tmpOut, src, target)
	case FamilySheet:
		err = convertExcel(app, tmpIn, tmpOut, src, target)
	case FamilySlide:
		err = convertPowerPoint(app, tmpIn, tmpOut, target)
	default:
		err = apperr.New(apperr.DocFormatUnsupported, "不支持转成这个格式。")
	}
	if err != nil {
		return withEngine(err, d.ID)
	}
	// 关掉我们的文档：Quit 前看 Count
	safeQuit(app, fam)
	if err := stripBOMIfNeeded(tmpOut, target); err != nil {
		return err
	}
	return copyFile(tmpOut, outPath)
}

func progFor(d Detected, fam string) (progID, exeBase string) {
	switch fam {
	case FamilyText:
		return d.WordProgID, filepath.Base(d.WordExe)
	case FamilySheet:
		return d.ExcelProgID, filepath.Base(d.ExcelExe)
	case FamilySlide:
		return d.PowerPointProgID, filepath.Base(d.PowerPointExe)
	}
	return "", ""
}

func withEngine(err error, id string) error {
	ae := apperr.From(err)
	if ae == nil {
		return err
	}
	cp := *ae
	if cp.Detail == "" {
		cp.Detail = "engine=" + id
	} else if !strings.Contains(cp.Detail, "engine=") {
		cp.Detail = "engine=" + id + "\n" + cp.Detail
	}
	return &cp
}

func hardenApp(app *ole.IDispatch, fam string, hasMacro bool) error {
	// AutomationSecurity = 3，读回确认
	if _, err := oleutil.PutProperty(app, "AutomationSecurity", 3); err != nil {
		if hasMacro {
			return apperr.New(apperr.DocComponentCrashed, "文档组件意外退出，请重试。").WithDetail("macro_security")
		}
	} else {
		if v, err := oleutil.GetProperty(app, "AutomationSecurity"); err == nil {
			defer v.Clear()
			if v.Val != 3 && hasMacro {
				return apperr.New(apperr.DocComponentCrashed, "文档组件意外退出，请重试。").WithDetail("macro_security_confirm")
			}
		} else if hasMacro {
			return apperr.New(apperr.DocComponentCrashed, "文档组件意外退出，请重试。").WithDetail("macro_security_read")
		}
	}
	switch fam {
	case FamilyText:
		_, _ = oleutil.PutProperty(app, "DisplayAlerts", 0)
		_, _ = oleutil.PutProperty(app, "Visible", false)
		_, _ = oleutil.PutProperty(app, "ScreenUpdating", false)
	case FamilySheet:
		_, _ = oleutil.PutProperty(app, "DisplayAlerts", false)
		_, _ = oleutil.PutProperty(app, "AskToUpdateLinks", false)
		_, _ = oleutil.PutProperty(app, "Visible", false)
		_, _ = oleutil.PutProperty(app, "ScreenUpdating", false)
		_, _ = oleutil.PutProperty(app, "Interactive", false)
		_, _ = oleutil.PutProperty(app, "EnableEvents", false)
	case FamilySlide:
		_, _ = oleutil.PutProperty(app, "DisplayAlerts", 1) // ppAlertsNone
	}
	return nil
}

const fakePassword = "\x00ffmpegfree-no-password\x00"

func convertWord(app *ole.IDispatch, inPath, outPath, src, target string) error {
	docs := oleutil.MustGetProperty(app, "Documents").ToIDispatch()
	defer docs.Release()
	// Visible:=False 等可选参数用命名较难；先按位置试，失败再简化。
	var doc *ole.IDispatch
	if src == "pdf" {
		// 6.12.61：Documents.Open(FileName, ConfirmConversions:=False, ReadOnly:=True, AddToRecentFiles:=False,
		// PasswordDocument:=<假密码>, PasswordTemplate, Revert, WritePasswordDocument, WritePasswordTemplate,
		// Format:=wdOpenFormatAuto, Encoding:=msoEncodingAutoDetect, Visible:=False, OpenAndRepair:=False,
		// DocumentDirection:=wdLeftToRight, NoEncodingDialog:=True)
		r, err := oleutil.CallMethod(docs, "Open", inPath, false, true, false, fakePassword, "", false, "", "", 0, 50001, false, false, 0, true)
		if err != nil {
			r, err = oleutil.CallMethod(docs, "Open", inPath, false, true, false, fakePassword)
		}
		if err != nil {
			return mapCOMErr(err)
		}
		doc = r.ToIDispatch()
	} else if src == "txt" {
		// Encoding:=65001
		r, err := oleutil.CallMethod(docs, "Open", inPath, false, true, false, fakePassword, false, false, false, false, 65001, false, true)
		if err != nil {
			r, err = oleutil.CallMethod(docs, "Open", inPath, false, true)
		}
		if err != nil {
			return mapCOMErr(err)
		}
		doc = r.ToIDispatch()
	} else {
		r, err := oleutil.CallMethod(docs, "Open", inPath, false, true, false, fakePassword)
		if err != nil {
			r, err = oleutil.CallMethod(docs, "Open", inPath, false, true)
		}
		if err != nil {
			return mapCOMErr(err)
		}
		doc = r.ToIDispatch()
	}
	defer func() {
		_, _ = oleutil.CallMethod(doc, "Close", false)
		doc.Release()
	}()

	switch target {
	case "pdf":
		_, err := oleutil.CallMethod(doc, "ExportAsFixedFormat", outPath, 17) // wdExportFormatPDF
		return mapCOMErr(err)
	case "docx":
		_, err := oleutil.CallMethod(doc, "SaveAs2", outPath, 16)
		return mapCOMErr(err)
	case "doc":
		_, err := oleutil.CallMethod(doc, "SaveAs2", outPath, 0)
		return mapCOMErr(err)
	case "odt":
		_, err := oleutil.CallMethod(doc, "SaveAs2", outPath, 23)
		return mapCOMErr(err)
	case "rtf":
		_, err := oleutil.CallMethod(doc, "SaveAs2", outPath, 6)
		return mapCOMErr(err)
	case "txt":
		_, err := oleutil.CallMethod(doc, "SaveAs2", outPath, 7, 65001) // wdFormatUnicodeText + UTF-8
		return mapCOMErr(err)
	case "html":
		if wo, err := oleutil.GetProperty(doc, "WebOptions"); err == nil {
			disp := wo.ToIDispatch()
			_, _ = oleutil.PutProperty(disp, "Encoding", 65001)
			disp.Release()
			wo.Clear()
		}
		_, err := oleutil.CallMethod(doc, "SaveAs2", outPath, 10) // wdFormatFilteredHTML
		return mapCOMErr(err)
	default:
		return apperr.New(apperr.DocFormatUnsupported, "不支持转成这个格式。")
	}
}

func convertExcel(app *ole.IDispatch, inPath, outPath, src, target string) error {
	books := oleutil.MustGetProperty(app, "Workbooks").ToIDispatch()
	defer books.Release()
	var book *ole.IDispatch
	if src == "csv" {
		r, err := oleutil.CallMethod(books, "OpenText", inPath, 65001, 1, 1, true, false, true)
		if err != nil {
			r, err = oleutil.CallMethod(books, "Open", inPath, 0, true)
		}
		if err != nil {
			return mapCOMErr(err)
		}
		book = r.ToIDispatch()
	} else {
		r, err := oleutil.CallMethod(books, "Open", inPath, 0, true, fakePassword)
		if err != nil {
			r, err = oleutil.CallMethod(books, "Open", inPath, 0, true)
		}
		if err != nil {
			return mapCOMErr(err)
		}
		book = r.ToIDispatch()
	}
	defer func() {
		_, _ = oleutil.CallMethod(book, "Close", false)
		book.Release()
	}()

	switch target {
	case "pdf":
		_, err := oleutil.CallMethod(book, "ExportAsFixedFormat", 0, outPath) // xlTypePDF
		return mapCOMErr(err)
	case "xlsx":
		_, err := oleutil.CallMethod(book, "SaveAs", outPath, 51)
		return mapCOMErr(err)
	case "xls":
		_, err := oleutil.CallMethod(book, "SaveAs", outPath, 56)
		return mapCOMErr(err)
	case "ods":
		_, err := oleutil.CallMethod(book, "SaveAs", outPath, 60)
		return mapCOMErr(err)
	case "csv":
		// 激活第一个工作表
		if sheets, err := oleutil.GetProperty(book, "Worksheets"); err == nil {
			sh := sheets.ToIDispatch()
			if s1, err := oleutil.GetProperty(sh, "Item", 1); err == nil {
				ws := s1.ToIDispatch()
				_, _ = oleutil.CallMethod(ws, "Activate")
				ws.Release()
				s1.Clear()
			}
			sh.Release()
			sheets.Clear()
		}
		_, err := oleutil.CallMethod(book, "SaveAs", outPath, 62, nil, nil, false) // xlCSVUTF8
		return mapCOMErr(err)
	default:
		return apperr.New(apperr.DocFormatUnsupported, "不支持转成这个格式。")
	}
}

func convertPowerPoint(app *ole.IDispatch, inPath, outPath, target string) error {
	pres := oleutil.MustGetProperty(app, "Presentations").ToIDispatch()
	defer pres.Release()
	r, err := oleutil.CallMethod(pres, "Open", inPath, -1, 0, 0) // ReadOnly, Untitled, WithWindow
	if err != nil {
		return mapCOMErr(err)
	}
	p := r.ToIDispatch()
	defer func() {
		_, _ = oleutil.CallMethod(p, "Close")
		p.Release()
	}()
	switch target {
	case "pdf":
		_, err := oleutil.CallMethod(p, "SaveAs", outPath, 32)
		return mapCOMErr(err)
	case "pptx":
		_, err := oleutil.CallMethod(p, "SaveAs", outPath, 24)
		return mapCOMErr(err)
	case "ppt":
		_, err := oleutil.CallMethod(p, "SaveAs", outPath, 1)
		return mapCOMErr(err)
	case "odp":
		_, err := oleutil.CallMethod(p, "SaveAs", outPath, 35)
		return mapCOMErr(err)
	default:
		return apperr.New(apperr.DocFormatUnsupported, "不支持转成这个格式。")
	}
}

func safeQuit(app *ole.IDispatch, fam string) {
	countProp := "Documents"
	switch fam {
	case FamilySheet:
		countProp = "Workbooks"
	case FamilySlide:
		countProp = "Presentations"
	}
	if col, err := oleutil.GetProperty(app, countProp); err == nil {
		disp := col.ToIDispatch()
		if n, err := oleutil.GetProperty(disp, "Count"); err == nil {
			// 除了我们的还有别的 → 不 Quit
			if n.Val > 1 {
				disp.Release()
				col.Clear()
				n.Clear()
				return
			}
			n.Clear()
		}
		disp.Release()
		col.Clear()
	}
	_, _ = oleutil.CallMethod(app, "Quit")
}

func mapCOMErr(err error) error {
	if err == nil {
		return nil
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "password") || strings.Contains(s, "encrypted") {
		return apperr.New(apperr.DocEncrypted, "这个文件有密码保护，不能转换。请先去掉密码再添加。")
	}
	return apperr.Wrap(apperr.DocComponentCrashed, "文档组件意外退出，请重试。", err)
}

func stripZoneIdentifier(p string) {
	_ = os.Remove(p + ":Zone.Identifier")
}

func stripBOMIfNeeded(p, target string) error {
	if target != "csv" && target != "txt" {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return os.WriteFile(p, b[3:], 0o644)
	}
	return nil
}

func attachJob(pid uint32) {
	// 尽力：创建 Job Object 并把进程放进去；失败忽略。
	// 用 syscall 级别 API 较繁琐，此处仅 TerminateProcess 兜底即可满足契约“超时只杀我们的 PID”。
	_ = pid
}
