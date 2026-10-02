// Package gateway 是统一网关:业务模块把路由注册进来,请求命中后交给业务函数。
// 它是唯一对外入口,承载公共关卡(中间件)与统一信封(common.Response)。
//
// Web 框架可配置(gateway.engine),默认/推荐 gin:
//   - gin(默认/推荐):Gin 引擎,原生路由,生态成熟;代价是**路由不可摘除**(组件下线不会摘路由);
//   - std:           自研动态路由,支持注册 / 摘除(组件下线自动摘路由)。
//
// 业务函数只面对框架无关的 gateway.Ctx。
package gateway

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"GoTenon"
	"StarDreamerChaosmos/common"
)

// ServiceName 是网关在上下文里的服务名(宿主预挂到 root)。
const ServiceName = "svc/gateway"

// Router 是网关门面:持有一个可切换的 web 框架后端 + 网关信箱 + 路由归属表。
type Router struct {
	mu     sync.RWMutex
	fw     Framework
	owners map[string][]GoTenon.Disposer // owner 组件名 -> 其路由的注销函数
	inbox  chan GoTenon.Message
}

// NewRouter 创建网关门面(默认 std 后端)。
func NewRouter() *Router {
	return &Router{
		fw:     newStdFramework(),
		owners: make(map[string][]GoTenon.Disposer),
		inbox:  make(chan GoTenon.Message, 256),
	}
}

// UseFramework 切换 web 框架后端(name: std | gin)。须在任何路由注册之前调用。
func (r *Router) UseFramework(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch strings.ToLower(name) {
	case "", "std", "own", "native":
		r.fw = newStdFramework()
	case "gin":
		r.fw = newGinFramework()
	default:
		return fmt.Errorf("gateway: 未知 web 框架 %q(期望 std|gin)", name)
	}
	return nil
}

// Framework 返回当前框架名。
func (r *Router) Framework() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.fw.Name()
}

// Submit 把一条协议消息投进网关信箱:入队即返回,不碰 Manager,装载期安全。
func (r *Router) Submit(msg GoTenon.Message) error {
	select {
	case r.inbox <- msg:
		return nil
	default:
		return errors.New("gateway: 信箱已满")
	}
}

// Handle 直接注册一条路由(不归属任何组件),返回注销函数。
func (r *Router) Handle(method, pattern string, h HandlerFunc) GoTenon.Disposer {
	r.mu.RLock()
	fw := r.fw
	r.mu.RUnlock()
	return fw.Register(method, pattern, h)
}

// addOwnerRoute 注册一条路由并记到 owner 名下(供消息注册用)。
func (r *Router) addOwnerRoute(owner, method, pattern string, h HandlerFunc) {
	r.mu.Lock()
	d := r.fw.Register(method, pattern, h)
	if owner != "" {
		r.owners[owner] = append(r.owners[owner], d)
	}
	r.mu.Unlock()
}

// RemoveOwner 摘除某组件注册的全部路由(其卸载时自动调用)。
func (r *Router) RemoveOwner(owner string) {
	if owner == "" {
		return
	}
	r.mu.Lock()
	ds := r.owners[owner]
	delete(r.owners, owner)
	r.mu.Unlock()
	for _, d := range ds {
		_ = d()
	}
}

// ServeHTTP 交给当前框架后端。
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	fw := r.fw
	r.mu.RUnlock()
	fw.ServeHTTP(w, req)
}

// ---- 统一信封输出 ----

// OK 成功:HTTP 200,信封 code=200。
func OK(c *Ctx, data any) {
	c.JSON(http.StatusOK, common.Response{Code: 200, Msg: "ok", Data: data})
}

// Fail 失败:HTTP 状态码与信封 code 一致,提示为中文。
func Fail(c *Ctx, status int, msg string) {
	c.JSON(status, common.Response{Code: status, Msg: msg})
}
