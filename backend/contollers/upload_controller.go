package contollers

import (
	"FFmpegFree/backend/utils"
	"FFmpegFree/backend/vo"
	"bytes"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

var convertingMutex = &sync.Mutex{}
var convertingFiles = make(map[vo.VideoInfo]*exec.Cmd)

func Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	println("到了")
	if err != nil {
		c.String(500, "上传文件出错")
		return
	}

	// 获取原始文件名（不含路径）
	filename := file.Filename
	ext := filepath.Ext(filename)
	base := filename[:len(filename)-len(ext)]

	// 构建目标路径，并检查是否已存在
	dstDir := "public/user/"
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

	// 保存文件
	dst := filepath.Join(dstDir, newFilename)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		c.String(500, "保存文件失败")
		return
	}

	// 返回结果
	m := make(map[string]string)
	m["fileName"] = newFilename
	m["code"] = "200"
	m["url"] = "http://localhost:8000/" + dst
	c.JSON(200, utils.Success(m))
}
func GetConvertingFiles(c *gin.Context) {
	convertingMutex.Lock()
	defer convertingMutex.Unlock()

	var files []vo.VideoInfo
	for file := range convertingFiles {
		files = append(files, file)
	}
	c.JSON(http.StatusOK, utils.Success(files))
}
func Selectvideofile(c *gin.Context) {
	dir := "public/user/"

	files, err := os.ReadDir(dir)
	if err != nil {
		c.String(500, "读取文件夹失败: %v", err)
		return
	}

	var videos []vo.VideoInfo

	for _, file := range files {
		if isVideoFile(file.Name()) {
			filePath := filepath.Join(dir, file.Name())
			fileInfo, _ := os.Stat(filePath)
			videoInfo := vo.VideoInfo{
				Name:     file.Name(),
				Url:      "http://localhost:8000/" + filePath,
				Date:     fileInfo.ModTime().Format("2006-01-02 15:04:05"),
				Duration: getVideoDuration(filePath), // 自定义函数获取视频时长
			}
			videos = append(videos, videoInfo)
		}
	}

	c.JSON(200, utils.Success(videos))
}
func getVideoDuration(filePath string) string {
	fmt.Println(filePath)
	cmd := exec.Command("ffmpeg", "-i", "./"+filePath)
	fmt.Print(cmd)
	var out bytes.Buffer
	cmd.Stderr = &out
	cmd.Run()

	// 后续解析逻辑不变
	output := out.String()
	start := "Duration: "
	end := ","
	i := bytes.Index([]byte(output), []byte(start))
	if i == -1 {
		return "unknown"
	}
	j := bytes.Index([]byte(output[i+len(start):]), []byte(end))
	if j == -1 {
		return "unknown"
	}
	duration := output[i+len(start) : i+len(start)+j]
	return duration
}
func isVideoFile(filename string) bool {
	ext := filepath.Ext(filename)
	switch ext {
	case ".mp4", ".avi", ".mkv", ".mov", ".flv", ".gif", "webm":
		return true
	default:
		return false
	}
}
func Convert(c *gin.Context) {
	var videoInfo vo.VideoInfo

	if err := c.ShouldBindJSON(&videoInfo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	inputPath := "public/user/" + videoInfo.Name
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件不存在"})
		return
	}

	ext := filepath.Ext(videoInfo.Name)
	base := videoInfo.Name[:len(videoInfo.Name)-len(ext)]
	outputDir := "public/converted/"
	os.MkdirAll(outputDir, os.ModePerm)

	var cmd *exec.Cmd
	var outputFilename string

	switch videoInfo.TargetFormat {
	case "mp4":
		outputFilename = base + ".mp4"
		cmd = exec.Command("./ffmpeg/ffmpeg", "-i", inputPath, "-c:v", "libx264", "-preset", "fast", "-crf", "23", "-c:a", "aac", filepath.Join(outputDir, outputFilename))
	case "avi":
		outputFilename = base + ".avi"
		cmd = exec.Command("./ffmpeg/ffmpeg", "-i", inputPath, "-c:v", "mpeg4", "-vtag", "DIVX", "-c:a", "ac3", filepath.Join(outputDir, outputFilename))
	case "mkv":
		outputFilename = base + ".mkv"
		cmd = exec.Command("./ffmpeg/ffmpeg", "-i", inputPath, "-c:v", "libx264", "-c:a", "copy", filepath.Join(outputDir, outputFilename))
	case "mov":
		outputFilename = base + ".mov"
		cmd = exec.Command("./ffmpeg/ffmpeg", "-i", inputPath, "-c:v", "libx264", "-f", "mov", filepath.Join(outputDir, outputFilename))
	case "flv":
		outputFilename = base + ".flv"
		cmd = exec.Command("./ffmpeg/ffmpeg", "-i", inputPath, "-c:v", "flv", "-b:v", "1M", "-c:a", "libmp3lame", filepath.Join(outputDir, outputFilename))
	case "gif":
		palettePath := filepath.Join(outputDir, base+".png")
		paletteCmd := exec.Command(
			"./ffmpeg/ffmpeg",
			"-i", inputPath,
			"-vf", "fps=10,scale=320:-1,palettegen",
			palettePath,
		)

		err := paletteCmd.Run()
		if err != nil {
			fmt.Printf("生成调色板失败: %s\n", err.Error())
			return
		}

		outputFilename = base + ".gif"
		cmd = exec.Command(
			"./ffmpeg/ffmpeg",
			"-i", inputPath,
			"-i", palettePath,
			"-lavfi", "fps=10,scale=320:-1 [x]; [x][1]paletteuse",
			filepath.Join(outputDir, outputFilename),
		)
	case "webm":
		outputFilename = base + ".webm"
		cmd = exec.Command(
			"./ffmpeg/ffmpeg",
			"-i", inputPath,
			"-c:v", "libvpx-vp9",
			"-b:v", "1M",
			"-c:a", "libopus",
			filepath.Join(outputDir, outputFilename),
		)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的格式"})
		return
	}

	// 添加到正在转换列表
	convertingMutex.Lock()
	convertingFiles[videoInfo] = cmd
	convertingMutex.Unlock()

	// 启动 goroutine 异步执行转换任务
	go func() {
		defer func() {
			// 转换结束后从列表中移除
			convertingMutex.Lock()
			delete(convertingFiles, videoInfo)
			convertingMutex.Unlock()
		}()

		err := cmd.Run()
		if err != nil {
			fmt.Printf("转换失败: %s\n", err.Error())
		} else {
			fmt.Printf("转换完成: %s -> %s\n", videoInfo.Name, outputFilename)

			convertedUpDir := "public/convertedUp/"
			os.MkdirAll(convertedUpDir, os.ModePerm)
			finalPath := filepath.Join(convertedUpDir, outputFilename)
			// 将文件从 converted 移动到 convertedUp
			err = os.Rename(outputDir+outputFilename, finalPath)
			if err != nil {
				fmt.Printf("移动文件失败: %s\n", err.Error())
			}
		}
	}()

	// 立即返回响应，表示任务已提交
	c.JSON(http.StatusOK, utils.Success(gin.H{
		"message": "转换任务已提交",
		"file":    videoInfo.Name,
		"status":  "processing",
	}))
}
func Convertup(c *gin.Context) {
	dir := "public/convertedUp/"
	files, err := os.ReadDir(dir)
	if err != nil {
		c.String(500, "读取文件夹失败: %v", err)
		return
	}

	var videos []vo.VideoInfo

	for _, file := range files {
		if isVideoFile(file.Name()) {
			filePath := filepath.Join(dir, file.Name())
			fileInfo, _ := os.Stat(filePath)
			videoInfo := vo.VideoInfo{
				Name:     file.Name(),
				Url:      "http://localhost:8000/" + filePath,
				Date:     fileInfo.ModTime().Format("2006-01-02 15:04:05"),
				Duration: getVideoDuration(filePath), // 自定义函数获取视频时长
			}
			videos = append(videos, videoInfo)
		}
	}

	c.JSON(200, utils.Success(videos))
}
func Download(c *gin.Context) {
	// 设置响应头，触发浏览器下载行为
	filePath := "public/convertedUp/"

	// 从查询参数中获取文件名
	name := c.Query("name")
	if name == "" {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "缺少文件名参数"})
		return
	}

	// 构建完整文件路径
	fullPath := filePath + name

	// 检查文件是否存在
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "文件不存在"})
		return
	}

	// 设置 Content-Disposition 头以触发下载
	c.Header("Content-Disposition", "attachment; filename="+name)
	fmt.Println("下载文件:", fullPath)
	// 提供文件下载
	c.File(fullPath)
}
func DeleteUpsc(c *gin.Context) {
	var videoInfo vo.VideoInfo

	if err := c.ShouldBindJSON(&videoInfo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 构建文件路径
	filePath := "public/convertedUp/" + videoInfo.Name

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件不存在"})
		return
	}

	// 删除文件
	err := os.Remove(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除文件失败: " + err.Error()})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, utils.Success(gin.H{
		"message": "文件删除成功",
		"file":    videoInfo.Name,
	}))
}
func DeleteUp(c *gin.Context) {
	var videoInfo vo.VideoInfo

	if err := c.ShouldBindJSON(&videoInfo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 构建文件路径
	filePath := "public/user/" + videoInfo.Name

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件不存在"})
		return
	}

	// 删除文件
	err := os.Remove(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除文件失败: " + err.Error()})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, utils.Success(gin.H{
		"message": "文件删除成功",
		"file":    videoInfo.Name,
	}))
}
