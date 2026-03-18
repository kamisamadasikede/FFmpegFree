package contollers

import (
	"FFmpegFree/backend/utils"
	"FFmpegFree/backend/vo"
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/go-pdf/fpdf"
	"github.com/xuri/excelize/v2"
)

var officeConvertingMutex = &sync.Mutex{}
var officeConvertingFiles = make(map[string]context.CancelFunc)

func resolveFontPath() (string, string) {
	paths := []struct {
		name string
		path string
	}{
		{"simhei", "C:/Windows/Fonts/simhei.ttf"},
		{"msyh", "C:/Windows/Fonts/msyh.ttf"},
		{"simsun", "C:/Windows/Fonts/simsun.ttf"},
		{"arialunicode", "/Library/Fonts/Arial Unicode.ttf"},
		{"arialunicode", "/Library/Fonts/Arial Unicode MS.ttf"},
		{"dejavu", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"},
	}

	for _, item := range paths {
		if _, err := os.Stat(item.path); err == nil {
			return item.name, item.path
		}
	}
	return "", ""
}

func newPdfDocument() *fpdf.Fpdf {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 18, 15)
	pdf.SetAutoPageBreak(true, 18)
	pdf.AddPage()

	fontName, fontPath := resolveFontPath()
	if fontName != "" && fontPath != "" {
		pdf.AddUTF8Font(fontName, "", fontPath)
		pdf.SetFont(fontName, "", 12)
	} else {
		pdf.SetFont("Helvetica", "", 12)
	}

	return pdf
}

func readZipFile(path string, name string) ([]byte, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name == name {
			rc, err := file.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("文件不存在: %s", name)
}

func listZipFiles(path string, prefix string) ([]string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var names []string
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, prefix) && strings.HasSuffix(file.Name, ".xml") {
			names = append(names, file.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func extractTextRuns(data []byte, lineBreakTag string) []string {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var lines []string
	var current strings.Builder

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return lines
		}
		switch val := tok.(type) {
		case xml.StartElement:
			if val.Name.Local == "t" {
				var text string
				if err := decoder.DecodeElement(&text, &val); err == nil {
					current.WriteString(text)
				}
			}
		case xml.EndElement:
			if val.Name.Local == lineBreakTag {
				trimmed := strings.TrimSpace(current.String())
				if trimmed != "" {
					lines = append(lines, trimmed)
				}
				current.Reset()
			}
		}
	}
	if trimmed := strings.TrimSpace(current.String()); trimmed != "" {
		lines = append(lines, trimmed)
	}
	return lines
}

func convertDocxToPDF(ctx context.Context, inputPath string, outputPath string) error {
	data, err := readZipFile(inputPath, "word/document.xml")
	if err != nil {
		return err
	}
	lines := extractTextRuns(data, "p")

	pdf := newPdfDocument()
	for _, line := range lines {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		pdf.MultiCell(0, 6, line, "", "L", false)
	}
	return pdf.OutputFileAndClose(outputPath)
}

func convertXlsxToPDF(ctx context.Context, inputPath string, outputPath string) error {
	book, err := excelize.OpenFile(inputPath)
	if err != nil {
		return err
	}
	defer func() { _ = book.Close() }()

	pdf := newPdfDocument()

	for _, sheet := range book.GetSheetList() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		pdf.SetFontSize(14)
		pdf.Cell(0, 8, "Sheet: "+sheet)
		pdf.Ln(10)
		pdf.SetFontSize(11)

		rows, err := book.GetRows(sheet)
		if err != nil {
			continue
		}
		for _, row := range rows {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			line := strings.Join(row, "    ")
			if strings.TrimSpace(line) == "" {
				pdf.Ln(4)
				continue
			}
			pdf.MultiCell(0, 6, line, "", "L", false)
		}
		pdf.AddPage()
	}

	return pdf.OutputFileAndClose(outputPath)
}

func convertPptxToPDF(ctx context.Context, inputPath string, outputPath string) error {
	slideFiles, err := listZipFiles(inputPath, "ppt/slides/slide")
	if err != nil {
		return err
	}

	pdf := newPdfDocument()
	for idx, slidePath := range slideFiles {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		data, err := readZipFile(inputPath, slidePath)
		if err != nil {
			continue
		}
		lines := extractTextRuns(data, "p")
		pdf.SetFontSize(14)
		pdf.Cell(0, 8, fmt.Sprintf("Slide %d", idx+1))
		pdf.Ln(10)
		pdf.SetFontSize(11)
		for _, line := range lines {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			pdf.MultiCell(0, 6, line, "", "L", false)
		}
		if idx < len(slideFiles)-1 {
			pdf.AddPage()
		}
	}
	return pdf.OutputFileAndClose(outputPath)
}

func convertOfficeToPDF(ctx context.Context, inputPath string, outputPath string, ext string) error {
	switch ext {
	case ".docx", ".doc":
		if ext == ".doc" {
			return fmt.Errorf("旧版 DOC 格式暂不支持纯 Go 转换")
		}
		return convertDocxToPDF(ctx, inputPath, outputPath)
	case ".xlsx", ".xls":
		if ext == ".xls" {
			return fmt.Errorf("旧版 XLS 格式暂不支持纯 Go 转换")
		}
		return convertXlsxToPDF(ctx, inputPath, outputPath)
	case ".pptx", ".ppt":
		if ext == ".ppt" {
			return fmt.Errorf("旧版 PPT 格式暂不支持纯 Go 转换")
		}
		return convertPptxToPDF(ctx, inputPath, outputPath)
	default:
		return fmt.Errorf("不支持的格式")
	}
}

func UploadOffice(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "上传文件出错: %v", err)
		return
	}

	filename := file.Filename
	ext := filepath.Ext(filename)
	base := filename[:len(filename)-len(ext)]

	if !isOfficeFile(ext) {
		c.String(http.StatusBadRequest, "不支持的文件格式，仅支持 Word、Excel、PowerPoint 文件")
		return
	}

	dstDir := "public/office/"
	os.MkdirAll(dstDir, os.ModePerm)

	newFilename := filename
	i := 1
	for {
		_, err := os.Stat(filepath.Join(dstDir, newFilename))
		if os.IsNotExist(err) {
			break
		}
		newFilename = fmt.Sprintf("%s-%d%s", base, i, ext)
		i++
	}

	dst := filepath.Join(dstDir, newFilename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.String(http.StatusInternalServerError, "保存文件失败")
		return
	}

	m := make(map[string]string)
	m["fileName"] = newFilename
	m["code"] = "200"
	m["url"] = "http://localhost:19200/" + filepath.ToSlash(dst)
	m["originalName"] = filename
	c.JSON(http.StatusOK, utils.Success(m))
}

func ConvertOfficeToPDF(c *gin.Context) {
	var officeInfo vo.OfficeInfo

	if err := c.ShouldBindJSON(&officeInfo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败: " + err.Error()})
		return
	}

	if officeInfo.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件名不能为空"})
		return
	}

	inputPath := "public/office/" + officeInfo.Name
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件不存在"})
		return
	}

	ext := filepath.Ext(officeInfo.Name)
	base := officeInfo.Name[:len(officeInfo.Name)-len(ext)]

	outputDir := "public/office/pdf/"
	os.MkdirAll(outputDir, os.ModePerm)

	outputFilename := base + ".pdf"
	outputPath := filepath.Join(outputDir, outputFilename)

	officeConvertingMutex.Lock()
	if _, exists := officeConvertingFiles[officeInfo.Name]; exists {
		officeConvertingMutex.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"error": "该文件正在转换中"})
		return
	}

	if ext == ".doc" || ext == ".xls" || ext == ".ppt" {
		officeConvertingMutex.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"error": "旧版 Office 格式暂不支持纯 Go 转换，请先另存为新版格式"})
		return
	}
	if !isOfficeFile(ext) {
		officeConvertingMutex.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的格式"})
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	officeConvertingFiles[officeInfo.Name] = cancel
	officeConvertingMutex.Unlock()

	go func() {
		defer func() {
			officeConvertingMutex.Lock()
			delete(officeConvertingFiles, officeInfo.Name)
			officeConvertingMutex.Unlock()
		}()
		err := convertOfficeToPDF(ctx, inputPath, outputPath, ext)

		if err != nil {
			fmt.Printf("Office转PDF失败: %s, 错误: %v\n", officeInfo.Name, err)
		} else {
			fmt.Printf("Office转PDF完成: %s -> %s\n", officeInfo.Name, outputFilename)
		}
	}()

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"message":    "转换任务已提交",
		"file":       officeInfo.Name,
		"outputFile": outputFilename,
		"outputPath": "http://localhost:19200/" + filepath.ToSlash(outputPath),
		"status":     "processing",
	}))
}

func GetOfficeFiles(c *gin.Context) {
	dir := "public/office/"

	files, err := os.ReadDir(dir)
	if err != nil {
		c.String(http.StatusInternalServerError, "读取文件夹失败: %v", err)
		return
	}

	var offices []vo.OfficeInfo

	for _, file := range files {
		if isOfficeFile(filepath.Ext(file.Name())) {
			filePath := filepath.Join(dir, file.Name())
			fileInfo, _ := os.Stat(filePath)
			officeInfo := vo.OfficeInfo{
				Name: file.Name(),
				Url:  "http://localhost:19200/" + filepath.ToSlash(filePath),
			}
			_ = fileInfo
			offices = append(offices, officeInfo)
		}
	}

	c.JSON(http.StatusOK, utils.Success(offices))
}

func GetConvertedPDFiles(c *gin.Context) {
	dir := "public/office/pdf/"

	files, err := os.ReadDir(dir)
	if err != nil {
		c.String(http.StatusInternalServerError, "读取文件夹失败: %v", err)
		return
	}

	var pdfFiles []vo.OfficeInfo

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".pdf" {
			filePath := filepath.Join(dir, file.Name())
			pdfInfo := vo.OfficeInfo{
				Name: file.Name(),
				Url:  "http://localhost:19200/" + filepath.ToSlash(filePath),
			}
			pdfFiles = append(pdfFiles, pdfInfo)
		}
	}

	c.JSON(http.StatusOK, utils.Success(pdfFiles))
}

func DownloadOfficePDF(c *gin.Context) {
	filePath := "public/office/pdf/"

	name := c.Query("name")
	if name == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "缺少文件名参数"})
		return
	}

	fullPath := filePath + name

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	c.Header("Content-Disposition", "attachment; filename="+name)
	c.File(fullPath)
}

func DeleteOfficeFile(c *gin.Context) {
	var officeInfo vo.OfficeInfo

	if err := c.ShouldBindJSON(&officeInfo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	filePath := "public/office/" + officeInfo.Name

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件不存在"})
		return
	}

	err := os.Remove(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除文件失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"message": "文件删除成功",
		"file":    officeInfo.Name,
	}))
}

func DeleteOfficePDF(c *gin.Context) {
	var officeInfo vo.OfficeInfo

	if err := c.ShouldBindJSON(&officeInfo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	filePath := "public/office/pdf/" + officeInfo.Name

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件不存在"})
		return
	}

	err := os.Remove(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除文件失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"message": "PDF文件删除成功",
		"file":    officeInfo.Name,
	}))
}

func StopOfficeConversion(c *gin.Context) {
	var officeInfo vo.OfficeInfo
	if err := c.ShouldBindJSON(&officeInfo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败"})
		return
	}

	officeConvertingMutex.Lock()
	defer officeConvertingMutex.Unlock()

	cancel, exists := officeConvertingFiles[officeInfo.Name]
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该文件未在转换中"})
		return
	}

	cancel()

	delete(officeConvertingFiles, officeInfo.Name)

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"message": "转换已终止",
		"file":    officeInfo.Name,
	}))
}

func isOfficeFile(ext string) bool {
	switch ext {
	case ".docx", ".doc", ".xlsx", ".xls", ".pptx", ".ppt":
		return true
	default:
		return false
	}
}
