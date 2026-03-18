package contollers

import (
	"FFmpegFree/backend/utils"
	"FFmpegFree/backend/vo"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func UploadPDF(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "上传文件出错: %v", err)
		return
	}

	filename := file.Filename
	ext := filepath.Ext(filename)

	if ext != ".pdf" {
		c.String(http.StatusBadRequest, "只支持 PDF 文件")
		return
	}

	dstDir := "public/pdf/"
	os.MkdirAll(dstDir, os.ModePerm)

	newFilename := filename
	i := 1
	for {
		_, err := os.Stat(filepath.Join(dstDir, newFilename))
		if os.IsNotExist(err) {
			break
		}
		newFilename = fmt.Sprintf("%s-%d%s", filename[:len(filename)-len(ext)], i, ext)
		i++
	}

	dst := filepath.Join(dstDir, newFilename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.String(http.StatusInternalServerError, "保存文件失败")
		return
	}

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"fileName": newFilename,
		"code":     "200",
		"url":      "http://localhost:19200/" + filepath.ToSlash(dst),
		"size":     file.Size,
	}))
}

func GetPDFFiles(c *gin.Context) {
	dir := "public/pdf/"

	files, err := os.ReadDir(dir)
	if err != nil {
		c.String(http.StatusInternalServerError, "读取文件夹失败: %v", err)
		return
	}

	var pdfs []vo.PDFInfo

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".pdf" {
			filePath := filepath.Join(dir, file.Name())
			fileInfo, _ := os.Stat(filePath)
			size := int64(0)
			if fileInfo != nil {
				size = fileInfo.Size()
			}
			pdfInfo := vo.PDFInfo{
				Name: file.Name(),
				Url:  "http://localhost:19200/" + filepath.ToSlash(filePath),
				Size: size,
			}
			pdfs = append(pdfs, pdfInfo)
		}
	}

	c.JSON(http.StatusOK, utils.Success(pdfs))
}

func DeletePDFFile(c *gin.Context) {
	var pdfInfo vo.PDFInfo

	if err := c.ShouldBindJSON(&pdfInfo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	filePath := "public/pdf/" + pdfInfo.Name

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
		"name":    pdfInfo.Name,
	}))
}
