package vo

// JsonFormatRequest JSON格式化请求结构体
type JsonFormatRequest struct {
	Json    string `json:"json"`    // 要格式化的JSON字符串
	Indent  int    `json:"indent"`  // 缩进空格数（默认4）
	Compact bool   `json:"compact"` // 是否压缩为单行
}

// JsonFormatResponse JSON格式化响应结构体
type JsonFormatResponse struct {
	Formatted string   `json:"formatted"` // 格式化后的JSON
	Error     string   `json:"error"`     // 错误信息
	ErrorPos  ErrorPos `json:"errorPos"`  // 错误位置
}

// ErrorPos JSON错误位置结构体
type ErrorPos struct {
	Line   int `json:"line"`   // 行号
	Column int `json:"column"` // 列号
}

// JsonCompareRequest JSON比对请求结构体
type JsonCompareRequest struct {
	Json1 string `json:"json1"` // 第一个JSON字符串
	Json2 string `json:"json2"` // 第二个JSON字符串
}

// JsonCompareResponse JSON比对响应结构体
type JsonCompareResponse struct {
	Identical   bool         `json:"identical"`   // 是否相同
	Differences []Difference `json:"differences"` // 差异列表
	Error       string       `json:"error"`       // 错误信息
	ErrorPos    ErrorPos     `json:"errorPos"`    // 错误位置
}

// Difference JSON差异结构体
type Difference struct {
	Type     string `json:"type"`     // 差异类型：added（新增）、removed（删除）、modified（修改）
	Path     string `json:"path"`     // 差异路径
	OldValue string `json:"oldValue"` // 旧值
	NewValue string `json:"newValue"` // 新值
}

// JsonValidateRequest JSON验证请求结构体
type JsonValidateRequest struct {
	Json string `json:"json"` // 要验证的JSON字符串
}

// JsonValidateResponse JSON验证响应结构体
type JsonValidateResponse struct {
	Valid    bool     `json:"valid"`    // 是否有效
	Error    string   `json:"error"`    // 错误信息
	ErrorPos ErrorPos `json:"errorPos"` // 错误位置
}
