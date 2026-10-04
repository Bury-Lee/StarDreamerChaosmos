// plugin.go —— 消息队列的 GoTenon 组件外壳:
// 作为内核消息的一个端点,统一处理 MQ 协议消息(TypeMQPublish / Subscribe / Unsubscribe),
// 并把内核生命周期事件(IndexEvent)桥接成队列主题。
package mq

import (
	"fmt"
	"sync"

	"GoTenon"
)

// Plugin 是消息队列组件外壳。
type Plugin struct {
	mu     sync.Mutex
	holder *Queue
}

// NewPlugin 创建消息队列组件。
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return PluginName }
func (p *Plugin) Desc() map[string]string {
	return map[string]string{"provides": ServiceName, "note": "内核消息队列:主题异步订阅/广播"}
}
func (p *Plugin) Inject() []string { return nil }

func (p *Plugin) Status() map[string]any {
	q := p.queue()
	if q == nil {
		return nil
	}
	topics, subs := q.Stats()
	m := map[string]any{"state": "ready", "topics": topics, "subscribers": subs, "pending": q.Pending()}
	return m
}

func (p *Plugin) Register() error { return nil }

// Apply 解析宿主预挂的队列容器。
func (p *Plugin) Apply(ctx *GoTenon.GoTenonContext, _ any) error {
	slot := ctx.SlotOf(ServiceName)
	if slot == nil || slot.Value == nil {
		return fmt.Errorf("mq: 宿主未预挂 %s", ServiceName)
	}
	q, ok := slot.Value.(*Queue)
	if !ok {
		return fmt.Errorf("mq: %s 类型不符: %T", ServiceName, slot.Value)
	}
	p.mu.Lock()
	p.holder = q
	p.mu.Unlock()
	fmt.Println("[mq] 消息队列就绪")
	return nil
}

// Run 启动消费者池(幂等):只起协程,不回调 Manager,装载期安全。
func (p *Plugin) Run() error {
	if q := p.queue(); q != nil {
		q.Start()
	}
	return nil
}

func (p *Plugin) Start() error { return nil }
func (p *Plugin) End() error   { return nil }

// DealWithMessage 是 MQ 协议的统一入口(内核按插件名 "mq" 路由到这里)。
//
// 处理:
//   - TypeMQPublish     :发布请求 → 入队(异步入队即返回)
//   - TypeMQSubscribe   :动态订阅
//   - TypeMQUnsubscribe :动态退订
//   - TypeIndex         :内核生命周期事件 → 桥接成队列主题
func (p *Plugin) DealWithMessage(m GoTenon.Message) error {
	switch m.Type {
	case TypeMQPublish:
		pub, ok := m.Data.(Publish)
		if !ok {
			return fmt.Errorf("mq: TypeMQPublish 载荷类型不符: %T", m.Data)
		}
		q := p.queue()
		if q == nil {
			return fmt.Errorf("mq: 队列未就绪")
		}
		return q.enqueue(pub.Topic, pub.Data)

	case TypeMQSubscribe:
		s, ok := m.Data.(Subscribe)
		if !ok {
			return fmt.Errorf("mq: TypeMQSubscribe 载荷类型不符: %T", m.Data)
		}
		if q := p.queue(); q != nil {
			q.addSub(s.Topic, s.Subscriber)
		}
		return nil

	case TypeMQUnsubscribe:
		s, ok := m.Data.(Subscribe)
		if !ok {
			return fmt.Errorf("mq: TypeMQUnsubscribe 载荷类型不符: %T", m.Data)
		}
		if q := p.queue(); q != nil {
			q.removeSub(s.Topic, s.Subscriber)
		}
		return nil

	case TypeMQForward:
		fw, ok := m.Data.(Forward)
		if !ok {
			return fmt.Errorf("mq: TypeMQForward 载荷类型不符: %T", m.Data)
		}
		q := p.queue()
		if q == nil {
			return fmt.Errorf("mq: 队列未就绪")
		}
		return q.forward(fw)

	case GoTenon.TypeIndex:
		ev, ok := m.Data.(GoTenon.IndexEvent)
		if !ok {
			return nil
		}
		q := p.queue()
		if q == nil {
			return nil
		}
		topic := TopicComponentChanged
		switch ev.Kind {
		case "up":
			topic = TopicComponentReady
		case "down":
			topic = TopicComponentGone
		}
		_ = q.enqueue(topic, ev) // 满则丢弃,不阻断内核
		return nil
	}
	return nil
}

func (p *Plugin) Function() map[string]any {
	return map[string]any{"mq.publish": map[string]any{"description": "向主题异步入队消息"}}
}
func (p *Plugin) ExecuteFunction(any) {}

func (p *Plugin) queue() *Queue {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.holder
}
