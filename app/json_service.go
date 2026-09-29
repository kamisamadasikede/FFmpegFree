package app

import "FFmpegFree/internal/service/jsontool"

// JsonService 是 JSON 工具的 Wails 绑定（契约第 4 节）。纯函数，不依赖存储。
type JsonService struct{}

func NewJsonService() *JsonService { return &JsonService{} }

// Format 格式化或压缩 JSON。
func (s *JsonService) Format(req jsontool.FormatRequest) (jsontool.FormatResponse, error) {
	return jsontool.Format(req)
}

// Compare 比对两段 JSON。
func (s *JsonService) Compare(req jsontool.CompareRequest) (jsontool.CompareResponse, error) {
	return jsontool.Compare(req)
}

// Validate 校验 JSON 语法。
func (s *JsonService) Validate(req jsontool.ValidateRequest) (jsontool.ValidateResponse, error) {
	return jsontool.Validate(req)
}
