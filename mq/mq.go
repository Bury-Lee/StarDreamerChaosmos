// Package mq 是内核消息队列组件:基于主题的异步订阅 / 广播。
//
// 与同步直接调用的区别:
//   - Publish 把消息封装成 TypeMQPublish 的内核消息交给 mq 组件(经 Manager.Send 路由);
//   - mq 组件把请求放入带缓冲的 channel 后立即返回(入队即返回);
//   - 后台消费者(worker)从队列取出,再以 TypeMQEvent 投递给订阅者;
//   - 生产者与消费者解耦,队列满时用背压(ErrQueueFull)而不是拖慢调用方。
//
// 用法:
//   - 宿主创建 *Queue 并预挂到 root 的 svc/mq,再 Bind(Manager);
//   - 组件在 Apply 期 Subscribe(topic, 自己的名字),Disposer 挂 ctx.Register 自动退订;
//   - 任一组件在运行期 Publish(topic, data);订阅者在 DealWithMessage 处理 mq.TypeMQEvent 的 mq.Event。
//
// 约束:发布/投递都经 Manager.Send,所以只能在运行期数据面发生;装载期调用 Publish 会因目标组件未就绪而失败。
package mq

import (
	"errors"
	"sort"
	"sync"

	"GoTenon"
)

// ServiceName 是消息队列在上下文里的服务名(宿主预挂到 root)。
const ServiceName = "svc/mq"

// 内核生命周期事件桥接出的主题。
const (
	TopicComponentReady   = "component.ready"   // 某组件装载完成(up)
	TopicComponentGone    = "component.gone"    // 某组件卸载(down)
	TopicComponentChanged = "component.changed" // 组件状态刷新(update)
)

// ErrQueueFull 是队列已满的背压信号:调用方可选择重试、丢弃或降级处理。
var ErrQueueFull = errors.New("mq: 队列已满")

// job 是队列内部的一条待投递消息。
type job struct {
	topic string
	data  any
}

// Queue 是主题订阅表 + 异步消息队列 + 消费者池。
type Queue struct {
	mu   sync.RWMutex
	m    *GoTenon.Manager
	subs map[string][]string // topic -> 订阅者组件名

	ch      chan job
	stop    chan struct{}
	workers int

	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup
}

// New 创建消息队列;buffer<=0 默认 256,workers<=0 默认 4。
func New(buffer, workers int) *Queue {
	if buffer <= 0 {
		buffer = 256
	}
	if workers <= 0 {
		workers = 4
	}
	return &Queue{
		subs:    make(map[string][]string),
		ch:      make(chan job, buffer),
		stop:    make(chan struct{}),
		workers: workers,
	}
}

// Bind 注入内核管理器(宿主在 NewManager 之后调用;发布与投递都依赖它)。
func (q *Queue) Bind(m *GoTenon.Manager) {
	q.mu.Lock()
	q.m = m
	q.mu.Unlock()
}

// Subscribe 让 subscriber 订阅 topic,返回幂等退订函数。
// 只改内存表,不碰 Manager,故装载期调用也安全。
func (q *Queue) Subscribe(topic, subscriber string) GoTenon.Disposer {
	q.addSub(topic, subscriber)
	return func() error {
		q.removeSub(topic, subscriber)
		return nil
	}
}

// Publish 通过内核消息把发布请求交给 mq 组件(标准化的 TypeMQPublish)。
// 入队是 O(1),真正的投递由后台消费者异步完成;队列满时返回 ErrQueueFull。
func (q *Queue) Publish(topic string, data any) error {
	q.mu.RLock()
	m := q.m
	q.mu.RUnlock()
	if m == nil {
		return errors.New("mq: Manager 未绑定(先 Bind)")
	}
	return m.Send(GoTenon.Message{Name: PluginName, Type: TypeMQPublish, Data: Publish{Topic: topic, Data: data}})
}

// Report 按项目规范上报一次状态变更(统一发布到 TopicStateChanged)。
// 组件的一切状态变更都应经此汇聚到 bus。
func (q *Queue) Report(component string, state any) error {
	return q.Publish(TopicStateChanged, StateChange{Component: component, State: state})
}

// Forward 请内核把一次调用转发给 target 组件(经 TypeMQForward 走 bus)。
// 组件无法直接访问 Manager,故用此表达"帮我调用 X"的意图。
func (q *Queue) Forward(target string, t GoTenon.MessageType, data any) error {
	q.mu.RLock()
	m := q.m
	q.mu.RUnlock()
	if m == nil {
		return errors.New("mq: Manager 未绑定(先 Bind)")
	}
	return m.Send(GoTenon.Message{Name: PluginName, Type: TypeMQForward, Data: Forward{Target: target, Type: t, Data: data}})
}

// forward 执行真正的转发(mq 组件收到 TypeMQForward 后调用)。
func (q *Queue) forward(fw Forward) error {
	q.mu.RLock()
	m := q.m
	q.mu.RUnlock()
	if m == nil {
		return errors.New("mq: Manager 未绑定(先 Bind)")
	}
	return m.Send(GoTenon.Message{Name: fw.Target, Type: fw.Type, Data: fw.Data})
}

// enqueue 是内部入队:供 mq 组件收到 TypeMQPublish 后调用,以及内核事件桥接。
func (q *Queue) enqueue(topic string, data any) error {
	select {
	case <-q.stop:
		return errors.New("mq: 已停止")
	default:
	}
	select {
	case q.ch <- job{topic: topic, data: data}:
		return nil
	default:
		return ErrQueueFull
	}
}

// Start 启动消费者池(幂等)。建议由组件 Run 调用;只起协程,不碰 Manager,装载期安全。
func (q *Queue) Start() {
	q.startOnce.Do(func() {
		for i := 0; i < q.workers; i++ {
			q.wg.Add(1)
			go q.worker()
		}
	})
}

// Shutdown 停止消费者并等待退出(进程退出 / 测试用;不要在组件卸载路径里调用)。
func (q *Queue) Shutdown() {
	q.stopOnce.Do(func() { close(q.stop) })
	q.wg.Wait()
}

func (q *Queue) worker() {
	defer q.wg.Done()
	for {
		select {
		case <-q.stop:
			return
		case j := <-q.ch:
			q.deliver(j)
		}
	}
}

// deliver 把一条消息以 TypeMQEvent 投递给该主题的全部订阅者。
func (q *Queue) deliver(j job) {
	q.mu.RLock()
	m := q.m
	subs := append([]string(nil), q.subs[j.topic]...)
	q.mu.RUnlock()
	if m == nil {
		return
	}
	msg := GoTenon.Message{Type: TypeMQEvent, Data: Event{Topic: j.topic, Data: j.data}}
	for _, name := range subs {
		msg.Name = name
		if err := m.Send(msg); err != nil {
			// 订阅者可能已卸载:忽略,避免一个坏订阅者阻断其余
			continue
		}
	}
}

// Pending 返回当前排队等待投递的消息数。
func (q *Queue) Pending() int { return len(q.ch) }

// Stats 返回主题数与订阅关系总数(诊断用)。
func (q *Queue) Stats() (topics, subscribers int) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	for _, list := range q.subs {
		if len(list) > 0 {
			topics++
		}
		subscribers += len(list)
	}
	return
}

// Topics 返回全部有订阅者的主题快照(诊断用)。
func (q *Queue) Topics() []string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	out := make([]string, 0, len(q.subs))
	for t, list := range q.subs {
		if len(list) > 0 {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

func (q *Queue) addSub(topic, subscriber string) {
	q.mu.Lock()
	if !contains(q.subs[topic], subscriber) {
		q.subs[topic] = append(q.subs[topic], subscriber)
	}
	q.mu.Unlock()
}

func (q *Queue) removeSub(topic, subscriber string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	list := q.subs[topic]
	for i, s := range list {
		if s == subscriber {
			q.subs[topic] = append(list[:i], list[i+1:]...)
			return
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
