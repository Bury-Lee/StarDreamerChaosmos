// Package cap 是"能力外借"的通信契约与协议。
//
// 为什么用回调而不是"再发一条应答消息":
//
//	消费者在 Apply/Start/Run 期间"自己还没进入内核 router"(router 在装载成功后才登记),
//	此时它收不到发给自己的消息。内嵌回调可绕开该限制,让装配期即可"发请求→当场拿到"。
//
// 时序兜底:若请求时提供方尚未上线,Forward 会失败(ErrNotProvided);
// 消费者订阅 `component.ready`,提供方上线后**重发请求**补取;`component.gone` 时清缓存。
//
// 约定:
//   - 消息载荷 `Fn any` 是 Go 函数指针,仅进程内有效;拆分部署时提供方应返回
//     "本地桩函数",内部再由拨号器走远程。
//   - 能力键与函数类型即契约,双方编译期对齐(见 auth.go)。
//   - 取接口是控制面(消息一次),调用是数据面(直调)。
package cap

import "GoTenon"

// TypeRequest 是能力请求消息的类型码(组件自定义段;避开 mq 的 16–31、gateway 的 40–41)。
const TypeRequest GoTenon.MessageType = GoTenon.TypeCustomBase + 48

// Request 是能力请求载荷。
type Request struct {
	Key   string      // 能力键
	ReqID uint64      // 请求序号(诊断/关联)
	Reply func(Reply) // 应答回调:提供方查表后同步调用(进程内;可为 nil 表示不回收)
}

// Reply 是应答内容,交回给 Request.Reply。
type Reply struct {
	Key   string // 能力键
	ReqID uint64 // 回带请求序号
	Fn    any    // 函数指针(进程内)
	Err   string // 失败原因(空 = 成功)
}

// Forwarder 是"把一条消息投给某组件"的最小能力(由 *mq.Queue 满足)。
type Forwarder interface {
	Forward(target string, t GoTenon.MessageType, data any) error
}
