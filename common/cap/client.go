package cap

import "sync"

// Client 是能力消费方:向提供方请求函数指针并缓存,供热路径直调。
//
// 用法(组件装配期):
//
//	c := cap.NewClient()
//	c.Bind(queue, "user", cap.KeyAuthRequireLogin)
//	requireLogin := c.Get(cap.KeyAuthRequireLogin).(cap.RequireLoginFunc)
//
// 因 Forward 同步投递、应答经请求内嵌回调交回,Bind 返回时缓存已就绪(提供方已上线时)。
// 提供方未上线则 Bind 拿不到;组件应订阅 component.ready 在其上线后重新 Bind,
// 并在 component.gone 时 Clear。
type Client struct {
	mu  sync.RWMutex
	fns map[string]any
	seq uint64
}

// NewClient 创建能力消费方。
func NewClient() *Client {
	return &Client{fns: make(map[string]any)}
}

// Bind 向 provider 请求一批能力;提供方已上线则返回时缓存已就绪。
func (c *Client) Bind(fwd Forwarder, provider string, keys ...string) {
	for _, k := range keys {
		c.seq++
		key := k
		req := Request{
			Key:   key,
			ReqID: c.seq,
			Reply: func(rep Reply) {
				if rep.Err == "" {
					c.store(rep.Key, rep.Fn)
				}
			},
		}
		_ = fwd.Forward(provider, TypeRequest, req)
	}
}

// Get 取一个已缓存的能力函数指针;未绑定返回 nil。调用方自行断言到具体函数类型。
func (c *Client) Get(key string) any {
	c.mu.RLock()
	fn := c.fns[key]
	c.mu.RUnlock()
	return fn
}

// Clear 清空全部缓存(提供方下线时调用,令后续调用降级/失败关闭)。
func (c *Client) Clear() {
	c.mu.Lock()
	c.fns = make(map[string]any)
	c.mu.Unlock()
}

// ClearKey 清空指定能力。
func (c *Client) ClearKey(key string) {
	c.mu.Lock()
	delete(c.fns, key)
	c.mu.Unlock()
}

func (c *Client) store(key string, fn any) {
	c.mu.Lock()
	c.fns[key] = fn
	c.mu.Unlock()
}
