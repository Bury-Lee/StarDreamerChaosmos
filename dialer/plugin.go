package dialer

import (
	"fmt"
	"sync"

	"GoTenon"
)

// TODO:之后将其Dialer变成一个抽象,以支持本地或远程的更好的优化,例如说带有支持配置中心,注册中心的一种,仅通过配置文件读取有什么地址(grpc服务器地址)可以提供微服务的一种等等.

// Plugin 是拨号器的组件外壳。
type Plugin struct {
	mu     sync.Mutex
	holder *Dialer
}

// NewPlugin 创建拨号器组件。
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "dialer" }
func (p *Plugin) Desc() map[string]string {
	return map[string]string{"provides": ServiceName, "作用": "全局拨号器,业务与微服务的中间层,为两边提供注册治理"}
}

// TODO:支持在配置中自定义一些行为等
func (p *Plugin) Inject() []string { return nil }

func (p *Plugin) Status() map[string]any {
	p.mu.Lock()
	d := p.holder
	p.mu.Unlock()
	if d == nil {
		return nil
	}
	m := map[string]any{"state": "ready", "conns": d.ConnCount()}
	return m
}

func (p *Plugin) Register() error { return nil }

func (p *Plugin) Apply(ctx *GoTenon.GoTenonContext, _ any) error {
	slot := ctx.SlotOf(ServiceName)
	if slot == nil || slot.Value == nil {
		return fmt.Errorf("dialer: 宿主未预挂 %s", ServiceName)
	}
	d, ok := slot.Value.(*Dialer)
	if !ok {
		return fmt.Errorf("dialer: %s 类型不符: %T", ServiceName, slot.Value)
	}
	p.mu.Lock()
	p.holder = d
	p.mu.Unlock()
	fmt.Println("[dialer] 就绪")
	return nil
}

func (p *Plugin) Start() error                          { return nil }
func (p *Plugin) Run() error                            { return nil }
func (p *Plugin) End() error                            { return nil }
func (p *Plugin) DealWithMessage(GoTenon.Message) error { return nil }
func (p *Plugin) Function() map[string]any              { return nil }
func (p *Plugin) ExecuteFunction(any)                   {}
