package mq

import "GoTenon"

// PluginName 是消息队列组件的插件名(内核按插件名路由消息,须与 Plugin.Name() 一致)。
const PluginName = "mq"

// MQ 保留的消息类型段:自 GoTenon.TypeCustomBase(16) 起,16–31 预留给消息队列。
//
// 标准化约定:
//   - 任何"投递给消息队列"的请求,都封装成 TypeMQPublish 的内核消息,
//     经 Manager.Send 路由到 mq 组件(发布也走内核消息,而不是旁路入队);
//   - mq 组件消费后,把事件以 TypeMQEvent 投递给订阅者;
//   - 动态订阅 / 退订分别是 TypeMQSubscribe / TypeMQUnsubscribe。
const (
	TypeMQPublish     GoTenon.MessageType = GoTenon.TypeCustomBase + iota // 16 发布:载荷 Publish
	TypeMQSubscribe                                                       // 17 动态订阅:载荷 Subscribe
	TypeMQUnsubscribe                                                     // 18 动态退订:载荷 Subscribe
	TypeMQEvent                                                           // 19 事件投递:载荷 Event
	TypeMQForward                                                         // 20 请内核转发调用:载荷 Forward
)

// Publish 是 TypeMQPublish 的载荷。
type Publish struct {
	Topic string
	Data  any
}

// Subscribe 是 TypeMQSubscribe / TypeMQUnsubscribe 的载荷。
type Subscribe struct {
	Topic      string
	Subscriber string
}

// Forward 是 TypeMQForward 的载荷:请内核把 Msg 转发给 Target 组件。
type Forward struct {
	Target string
	Type   GoTenon.MessageType
	Data   any
}

// Event 是 TypeMQEvent 的载荷(队列投递给订阅者)。
type Event struct {
	Topic string // 主题
	Data  any    // 载荷
}

// TopicStateChanged 是所有组件上报"状态变更"的统一主题。
// 项目规范:任何项目状态等变更都应向 bus 发消息,统一汇聚到这里。
const TopicStateChanged = "state.changed"

// StateChange 是发布到 TopicStateChanged 的推荐载荷。
type StateChange struct {
	Component string // 上报组件名
	State     any    // 状态内容(自定义)
}
