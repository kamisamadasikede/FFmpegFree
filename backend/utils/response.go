package utils

type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Success 返回成功响应
func Success(data interface{}) Response {
	return Response{
		Code:    200,
		Message: "操作成功",
		Data:    data,
	}
}

func Fail(code int, message string) Response {
	return Response{
		Code:    code,
		Message: message,
		Data:    nil,
	}
}
