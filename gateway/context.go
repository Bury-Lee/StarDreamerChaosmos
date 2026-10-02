package gateway

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

// HandlerFunc 是框架无关的处理函数;业务函数只面对 *Ctx。
type HandlerFunc func(*Ctx)

// Ctx 是框架无关的请求上下文:同时适配自研路由(net/http)与 Gin。
type Ctx struct {
	gc *gin.Context     // gin 后端时非空
	w  http.ResponseWriter
	r  *http.Request
}

// BindJSON 解析 JSON 请求体。
func (c *Ctx) BindJSON(v any) error {
	if c.gc != nil {
		return c.gc.ShouldBindJSON(v)
	}
	return json.NewDecoder(c.r.Body).Decode(v)
}

// JSON 输出 JSON 响应。
func (c *Ctx) JSON(status int, v any) {
	if c.gc != nil {
		c.gc.JSON(status, v)
		return
	}
	c.w.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.w.WriteHeader(status)
	_ = json.NewEncoder(c.w).Encode(v)
}

// Context 返回请求的 context(用于下游 RPC)。
func (c *Ctx) Context() context.Context { return c.r.Context() }

// Param 取路径参数(自研路由暂不支持,返回空)。
func (c *Ctx) Param(key string) string {
	if c.gc != nil {
		return c.gc.Param(key)
	}
	return ""
}

// Query 取查询参数。
func (c *Ctx) Query(key string) string { return c.r.URL.Query().Get(key) }

// Request 返回原始请求。
func (c *Ctx) Request() *http.Request { return c.r }
