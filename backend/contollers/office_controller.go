package contollers

import (
	"FFmpegFree/backend/utils"
	"FFmpegFree/backend/vo"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/gin-gonic/gin"
)

var officeConvertingMutex = &sync.Mutex{}
var officeConvertingFiles = make(map[string]*exec.Cmd)

// findLibreOffice 查找 LibreOffice 可执行文件路径
func findLibreOffice() string {
	possiblePaths := []string{
		"./libreoffice/bin/soffice",
		"./libreoffice/bin/soffice.exe",
		"C:/Program Files/LibreOffice/program/soffice.exe",
		"C:/Program Files (x86)/LibreOffice/program/soffice.exe",
		"/usr/bin/soffice",
		"/Applications/LibreOffice.app/Contents/MacOS/soffice",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
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
	m["url"] = "http://localhost:19200/" + dst
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

	loPath := findLibreOffice()
	if loPath == "" {
		officeConvertingMutex.Unlock()
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "未找到 LibreOffice，请安装 LibreOffice 并确保程序可访问。Windows 用户建议安装到默认路径或放置 soffice.exe 到项目根目录的 libreoffice/bin/ 文件夹下",
		})
		return
	}

	var cmd *exec.Cmd

	switch ext {
	case ".docx", ".doc":
		cmd = exec.Command(
			loPath,
			"--headless",
			"--convert-to",
			"pdf",
			"--outdir",
			outputDir,
			inputPath,
		)
	case ".xlsx", ".xls":
		cmd = exec.Command(
			loPath,
			"--headless",
			"--convert-to",
			"pdf",
			"--outdir",
			outputDir,
			inputPath,
		)
	case ".pptx", ".ppt":
		cmd = exec.Command(
			loPath,
			"--headless",
			"--convert-to",
			"pdf",
			"--outdir",
			outputDir,
			inputPath,
		)
	default:
		officeConvertingMutex.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的格式"})
		return
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	officeConvertingFiles[officeInfo.Name] = cmd
	officeConvertingMutex.Unlock()

	go func() {
		defer func() {
			officeConvertingMutex.Lock()
			delete(officeConvertingFiles, officeInfo.Name)
			officeConvertingMutex.Unlock()
		}()

		err := cmd.Run()

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
		"outputPath": "http://localhost:19200/" + outputPath,
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
				Url:  "http://localhost:19200/" + filePath,
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
				Url:  "http://localhost:19200/" + filePath,
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

	cmd, exists := officeConvertingFiles[officeInfo.Name]
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该文件未在转换中"})
		return
	}

	if cmd.Process != nil {
		if err := cmd.Process.Kill(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "终止转换失败"})
			return
		}
	}

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
