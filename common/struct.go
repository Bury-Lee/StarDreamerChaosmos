package common

// Response 是统一信封:状态码 + 数据 + 提示。
type Response struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}
