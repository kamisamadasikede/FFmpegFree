package vo

// OfficeInfo Office文件转PDF请求结构体
type OfficeInfo struct {
	Name         string `json:"name"`          // 文件名
	Url          string `json:"url"`           // 文件URL
	TargetFormat string `json:"targetFormat"` // 目标格式（默认pdf）
}
