package gateway

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"GoTenon"
)

// Framework 是 web 框架后端抽象。
type Framework interface {
	Name() string
	// Register 注册一条路由,返回幂等注销函数(不支持摘除的后端返回错误 Disposer)。
	Register(method, pattern string, h HandlerFunc) GoTenon.Disposer
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// ---- std:自研动态路由(net/http),支持注册 / 摘除 ----

type stdFramework struct {
	mu     sync.RWMutex
	routes []*stdRoute
	nextID int
}

type stdRoute struct {
	id      int
	method  string
	pattern string
	handler HandlerFunc
}

func newStdFramework() *stdFramework { return &stdFramework{} }

func (f *stdFramework) Name() string { return "std" }

func (f *stdFramework) Register(method, pattern string, h HandlerFunc) GoTenon.Disposer {
	f.mu.Lock()
	id := f.nextID
	f.nextID++
	f.routes = append(f.routes, &stdRoute{id: id, method: strings.ToUpper(method), pattern: pattern, handler: h})
	f.mu.Unlock()
	return func() error {
		f.mu.Lock()
		defer f.mu.Unlock()
		for i, rt := range f.routes {
			if rt.id == id {
				f.routes = append(f.routes[:i], f.routes[i+1:]...)
				break
			}
		}
		return nil
	}
}

func (f *stdFramework) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.RLock()
	routes := append([]*stdRoute(nil), f.routes...)
	f.mu.RUnlock()

	var h HandlerFunc = func(c *Ctx) { Fail(c, http.StatusNotFound, "未找到该接口") }
	for _, rt := range routes {
		if rt.method == r.Method && match(rt.pattern, r.URL.Path) {
			h = rt.handler
			break
		}
	}
	h(&Ctx{w: w, r: r})
}

// match 支持精确匹配与尾部通配:"/api/*" 命中 "/api/" 下任意路径。
func match(pattern, path string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(path, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == path
}

// ---- gin:Gin 引擎(原生路由,不可摘除) ----

type ginFramework struct {
	e *gin.Engine
}

func newGinFramework() *ginFramework {
	gin.SetMode(gin.ReleaseMode)
	e := gin.New()
	e.Use(gin.Recovery())
	e.NoRoute(func(gc *gin.Context) {
		Fail(&Ctx{gc: gc, r: gc.Request}, http.StatusNotFound, "未找到该接口")
	})
	return &ginFramework{e: e}
}

func (f *ginFramework) Name() string { return "gin" }

func (f *ginFramework) Register(method, pattern string, h HandlerFunc) GoTenon.Disposer {
	f.e.Handle(method, pattern, func(gc *gin.Context) { h(&Ctx{gc: gc, r: gc.Request}) })
	return func() error { return errors.New("gateway: gin 后端不支持摘除路由") }
}

func (f *ginFramework) ServeHTTP(w http.ResponseWriter, r *http.Request) { f.e.ServeHTTP(w, r) }
