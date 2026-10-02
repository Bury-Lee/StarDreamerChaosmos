package gateway

import "GoTenon"

// 网关协议消息类型(应用级,自 TypeCustomBase 起;MQ 占 16–31,这里避开)。
const (
	TypeRouteRegister   GoTenon.MessageType = GoTenon.TypeCustomBase + 40
	TypeRouteUnregister GoTenon.MessageType = GoTenon.TypeCustomBase + 41
)

// Route 是路由注册消息的载荷。
// Handler 是函数指针,仅进程内有效;Owner 是注册者组件名,用于其卸载时自动摘除。
type Route struct {
	Owner   string
	Method  string
	Pattern string
	Handler HandlerFunc
}

// RouteMessage 构造一条路由注册消息。
func RouteMessage(owner, method, pattern string, h HandlerFunc) GoTenon.Message {
	return GoTenon.Message{
		Type: TypeRouteRegister,
		Data: Route{Owner: owner, Method: method, Pattern: pattern, Handler: h},
	}
}
