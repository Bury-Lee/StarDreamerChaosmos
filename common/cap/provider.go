package cap

import (
	"sync"

	"GoTenon"
)

// Provider 是能力提供方:登记「能力键 → 函数指针」,并处理消费者的请求消息。
// 提供方组件在 Apply 里 Provide 自己的能力,并在 DealWithMessage 里调用 Handled。
type Provider struct {
	mu  sync.RWMutex
	fns map[string]any
}

// NewProvider 创建能力提供方。
func NewProvider() *Provider {
	return &Provider{fns: make(map[string]any)}
}

// Provide 登记一个能力函数(幂等覆盖)。
func (p *Provider) Provide(key string, fn any) {
	p.mu.Lock()
	p.fns[key] = fn
	p.mu.Unlock()
}

// Revoke 撤回一个能力。
func (p *Provider) Revoke(key string) {
	p.mu.Lock()
	delete(p.fns, key)
	p.mu.Unlock()
}

func (p *Provider) get(key string) (any, bool) {
	p.mu.RLock()
	fn, ok := p.fns[key]
	p.mu.RUnlock()
	return fn, ok
}

// Handled 报告该消息是否是能力请求并已处理(供组件在 DealWithMessage 里分流)。
// 处理:查表 → 经请求内嵌的回调同步把函数指针交回请求方。
func (p *Provider) Handled(msg GoTenon.Message) bool {
	if msg.Type != TypeRequest {
		return false
	}
	req, ok := msg.Data.(Request)
	if !ok {
		return true // 是本类消息但载荷不符,视为已处理
	}
	rep := Reply{Key: req.Key, ReqID: req.ReqID}
	if fn, found := p.get(req.Key); found {
		rep.Fn = fn
	} else {
		rep.Err = "capability not found: " + req.Key
	}
	if req.Reply != nil {
		req.Reply(rep)
	}
	return true
}
