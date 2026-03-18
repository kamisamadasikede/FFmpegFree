package vo

// PDFInfo PDF文件结构体
type PDFInfo struct {
	Name string `json:"name"` // 文件名
	Url  string `json:"url"`  // 文件URL
	Size int64  `json:"size"`
}
