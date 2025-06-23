package contollers

import (
	"FFmpegFree/backend/sse"
	"FFmpegFree/backend/utils"
	"FFmpegFree/backend/vo"
	"bytes"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/goccy/go-json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
)

var convertingMutex = &sync.Mutex{}
var streamsMutex = &sync.Mutex{}
var convertingFiles = make(map[vo.VideoInfo]*exec.Cmd)
var streams = make(map[vo.VideoInfo]*exec.Cmd)

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
func UploadSteamup(c *gin.Context) {
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
	dstDir := "public/steam/"
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
func GetSteamFiles(c *gin.Context) {
	dir := "public/steam/"

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
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow: true,
		}
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
func RemoveConvertingTask(c *gin.Context) {
	var videoInfo vo.VideoInfo
	if err := c.ShouldBindJSON(&videoInfo); err != nil {
		c.JSON(200, utils.Fail(500, "参数解析失败"))
		return
	}

	streamsMutex.Lock()
	defer streamsMutex.Unlock()

	// 查找是否正在转换的文件
	cmd, exists := convertingFiles[videoInfo]
	if !exists {
		c.JSON(200, utils.Fail(500, "查找失败"))
		return
	}

	// 终止进程
	if cmd.Process != nil {
		if err := cmd.Process.Kill(); err != nil {
			c.JSON(200, utils.Fail(500, "终止失败"))
			return
		}
	}

	// 从 map 中移除
	delete(streams, videoInfo)

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"message": "终止转换成功",
		"file":    videoInfo.Name,
	}))

}
func Steamload(c *gin.Context) {
	var videoInfo vo.VideoInfo

	if err := c.ShouldBindJSON(&videoInfo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败: " + err.Error()})
		return
	}

	// 检查是否提供了文件名和推流地址
	if videoInfo.Name == "" || videoInfo.SteamUrl == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少文件名或推流地址"})
		return
	}

	// 构建完整输入路径
	inputPath := "public/steam/" + videoInfo.Name

	// 检查文件是否存在
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件不存在"})
		return
	}

	// 检查是否是视频文件
	if !isVideoFile(videoInfo.Name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "非法文件类型"})
		return
	}

	// 构建 ffmpeg 命令
	cmd := exec.Command(
		"./ffmpeg/ffmpeg.exe",
		"-re", "-i", inputPath,
		"-c:v", "copy",
		"-c:a", "aac",
		"-f", "flv",
		videoInfo.SteamUrl,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	// 启动推流
	cmd.Start()
	/*	err := cmd.Start()
		if err != nil {
			// 推流启动失败，直接返回错误
			streamErr := fmt.Sprintf("推流启动失败：%v", err)
			if exitErr, ok := err.(*exec.ExitError); ok {
				streamErr += fmt.Sprintf(", 退出码: %d", exitErr.ExitCode())
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":     streamErr,
				"streamUrl": videoInfo.SteamUrl,
			})
			return
		}*/

	// 只有推流启动成功才记录到 map
	streamsMutex.Lock()
	streams[videoInfo] = cmd
	streamsMutex.Unlock()

	// 异步等待推流结束
	go func() {
		err := cmd.Wait()
		var status string
		var errorMsg string

		if err != nil {
			status = "failed"
			errorMsg = fmt.Sprintf("推流意外终止：%s，错误：%v\n", videoInfo.Name, err)
		} else {
			status = "completed"
			errorMsg = fmt.Sprintf("推流正常结束：%s\n", videoInfo.Name)
		}

		// 构造事件数据
		eventData := map[string]interface{}{
			"filename":  videoInfo.Name,
			"streamUrl": videoInfo.SteamUrl,
			"status":    status,
		}

		if errorMsg != "" {
			eventData["error"] = errorMsg
		}

		// 使用 SSE 广播事件
		jsonData, _ := json.Marshal(eventData)
		sse.BroadcastMessage(string(jsonData))

		// 清理 map
		streamsMutex.Lock()
		delete(streams, videoInfo)
		streamsMutex.Unlock()
	}()

	// 返回成功响应
	c.JSON(http.StatusOK, utils.Success(gin.H{
		"message": "推流任务已启动",
		"file":    videoInfo.Name,
		"stream":  videoInfo.SteamUrl,
	}))
}
func StopStream(c *gin.Context) {
	var videoInfo vo.VideoInfo
	if err := c.ShouldBindJSON(&videoInfo); err != nil {
		c.JSON(200, utils.Fail(500, "参数解析失败"))
		return
	}

	streamsMutex.Lock()
	defer streamsMutex.Unlock()

	// 查找是否正在推流该文件
	cmd, exists := streams[videoInfo]
	if !exists {
		c.JSON(200, utils.Fail(500, "该文件未在推流中"))
		return
	}

	// 终止推流进程
	if cmd.Process != nil {
		if err := cmd.Process.Kill(); err != nil {
			c.JSON(200, utils.Fail(500, "终止推流失败"))
			return
		}
	}

	// 从 map 中移除
	delete(streams, videoInfo)

	c.JSON(http.StatusOK, utils.Success(gin.H{
		"message": "推流已成功终止",
		"file":    videoInfo.Name,
	}))
}

func GetStreamingFiles(c *gin.Context) {
	streamsMutex.Lock()
	defer streamsMutex.Unlock()

	var activeStreams []vo.VideoInfo
	for filename := range streams {
		/*filename.SteamUrl = convertStreamURL(filename.SteamUrl)*/
		activeStreams = append(activeStreams, filename)
	}

	c.JSON(http.StatusOK, gin.H{
		"count":   len(activeStreams),
		"streams": activeStreams,
	})
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
func DeletesteamVideo(c *gin.Context) {
	var videoInfo vo.VideoInfo

	if err := c.ShouldBindJSON(&videoInfo); err != nil {
		c.JSON(200, utils.Fail(500, "参数错误"))
		return
	}

	// 构建文件路径
	filePath := "public/steam/" + videoInfo.Name

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(200, utils.Fail(500, "文件不存在"))
		return
	}

	// 删除文件
	err := os.Remove(filePath)
	if err != nil {
		c.JSON(200, utils.Fail(500, "删除文件失败: "+"当前视频真正被推流无法删除"))
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, utils.Success(gin.H{
		"message": "文件删除成功",
		"file":    videoInfo.Name,
	}))
}
