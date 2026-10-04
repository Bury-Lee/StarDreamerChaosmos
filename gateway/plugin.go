package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"GoTenon"
	"StarDreamerChaosmos/core/config"
	"StarDreamerChaosmos/utils"
)

// Plugin 是网关组件外壳:只依赖 config,从配置里读自身设置(addr / engine);
// 消费协议消息(路由注册)与内核生命周期事件(组件下线自动摘路由)。
type Plugin struct {
	mu     sync.Mutex
	router *Router
	addr   string
	srv    *http.Server
	ln     net.Listener
	served bool
}

// NewPlugin 创建网关组件。
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "gateway" }
func (p *Plugin) Desc() map[string]string {
	return map[string]string{
		"provides": ServiceName,
		"inject":   "config",
		"功能":       "全局统一网关,为插件提供路由注册接口,插件注册路由即可对外提供 HTTP 服务。推荐 gin(生态成熟;代价:路由不可摘除);需热摘除路由用 std",
	}
}

// Inject 只依赖 config:网关的全部设置来自配置。
func (p *Plugin) Inject() []string { return []string{"config"} }

func (p *Plugin) Status() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.router == nil {
		return nil
	}
	return map[string]any{"state": "ready", "addr": p.addr, "engine": p.router.Framework()}
}

func (p *Plugin) Register() error { return nil }

// Apply 取共享路由表,并按 config 的 gateway 段选择框架与监听地址。
func (p *Plugin) Apply(ctx *GoTenon.GoTenonContext, _ any) error {
	slot := ctx.SlotOf(ServiceName)
	if slot == nil || slot.Value == nil {
		return fmt.Errorf("gateway: 宿主未预挂 %s", ServiceName)
	}
	r, ok := slot.Value.(*Router)
	if !ok {
		return fmt.Errorf("gateway: %s 类型不符: %T", ServiceName, slot.Value)
	}

	// 从 config 读自身设置
	sec := map[string]any{}
	if cs := ctx.SlotOf(config.ServiceName); cs != nil && cs.Value != nil {
		if cfg, ok := cs.Value.(*config.Config); ok {
			sec, _ = cfg.Snapshot("gateway").(map[string]any)
		}
	}
	if err := r.UseFramework(utils.Str(sec, "engine", "gin")); err != nil {
		return err
	}

	p.mu.Lock()
	p.router = r
	p.addr = utils.Str(sec, "addr", "127.0.0.1:18080")
	p.mu.Unlock()
	return nil
}

// Run 只监听、不开始接收请求(只起协程,不回调 Manager,装载期安全)。
// 真正的 Serve 由宿主在装配完成后调 StartServing —— 先把已入队的路由消息落地,
// 再开始接收,避免"边服务边加路由"的竞态(尤其 gin)。
func (p *Plugin) Run() error {
	p.mu.Lock()
	addr := p.addr
	r := p.router
	p.mu.Unlock()

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("gateway: 监听 %s 失败: %w", addr, err)
	}
	p.mu.Lock()
	p.ln = ln
	p.srv = &http.Server{Handler: r}
	p.mu.Unlock()

	fmt.Printf("[gateway] 监听 %s(engine=%s)\n", addr, r.Framework())
	return nil
}

// StartServing 排空信箱里已到的路由消息,再开始接收请求。
func (p *Plugin) StartServing() {
	p.mu.Lock()
	r, srv, ln := p.router, p.srv, p.ln
	if p.served || srv == nil || ln == nil {
		p.mu.Unlock()
		return
	}
	p.served = true
	p.mu.Unlock()

	for {
		select {
		case msg := <-r.inbox:
			_ = p.DealWithMessage(msg)
		default:
			go func() { _ = srv.Serve(ln) }()
			return
		}
	}
}

func (p *Plugin) Start() error { return nil }

// End 优雅停机。
func (p *Plugin) End() error {
	p.mu.Lock()
	srv := p.srv
	p.mu.Unlock()
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
	return nil
}

// DealWithMessage 处理协议消息与内核生命周期事件。
func (p *Plugin) DealWithMessage(msg GoTenon.Message) error {
	p.mu.Lock()
	r := p.router
	p.mu.Unlock()
	if r == nil {
		return nil
	}

	switch msg.Type {
	case TypeRouteRegister:
		rt, ok := msg.Data.(Route)
		if !ok {
			return fmt.Errorf("gateway: TypeRouteRegister 载荷类型不符: %T", msg.Data)
		}
		r.addOwnerRoute(rt.Owner, rt.Method, rt.Pattern, rt.Handler)
		fmt.Printf("[gateway] 注册路由 %-4s %-22s (owner=%s)\n", rt.Method, rt.Pattern, rt.Owner)

	case TypeRouteUnregister:
		rt, ok := msg.Data.(Route)
		if !ok {
			return fmt.Errorf("gateway: TypeRouteUnregister 载荷类型不符: %T", msg.Data)
		}
		r.RemoveOwner(rt.Owner)
		fmt.Printf("[gateway] 摘除路由 (owner=%s)\n", rt.Owner)

	case GoTenon.TypeIndex:
		ev, ok := msg.Data.(GoTenon.IndexEvent)
		if ok && ev.Kind == "down" {
			r.RemoveOwner(ev.Plugin)
			fmt.Printf("[gateway] 组件 %s 下线,自动摘除其路由\n", ev.Plugin)
		}
	}
	return nil
}

func (p *Plugin) Function() map[string]any { return nil }
func (p *Plugin) ExecuteFunction(any)      {}
