package cap_test

import (
	"testing"

	"GoTenon"
	"StarDreamerChaosmos/common/cap"
	"StarDreamerChaosmos/mq"
)

// providerPlugin 提供能力 test.add。
type providerPlugin struct{ p *cap.Provider }

func (x *providerPlugin) Name() string            { return "provider" }
func (x *providerPlugin) Desc() map[string]string { return nil }
func (x *providerPlugin) Inject() []string        { return nil }
func (x *providerPlugin) Status() *map[string]any { return nil }
func (x *providerPlugin) Register() error         { return nil }
func (x *providerPlugin) Apply(*GoTenon.GoTenonContext, any) error {
	x.p = cap.NewProvider()
	x.p.Provide("test.add", func(a, b int) int { return a + b })
	return nil
}
func (x *providerPlugin) Start() error { return nil }
func (x *providerPlugin) Run() error   { return nil }
func (x *providerPlugin) DealWithMessage(m GoTenon.Message) error {
	x.p.Handled(m)
	return nil
}
func (x *providerPlugin) End() error { return nil }

// consumerPlugin 在装载期向 provider 请求 test.add。
type consumerPlugin struct{ got any }

func (x *consumerPlugin) Name() string            { return "consumer" }
func (x *consumerPlugin) Desc() map[string]string { return nil }
func (x *consumerPlugin) Inject() []string        { return nil }
func (x *consumerPlugin) Status() *map[string]any { return nil }
func (x *consumerPlugin) Register() error         { return nil }
func (x *consumerPlugin) Apply(ctx *GoTenon.GoTenonContext, _ any) error {
	q := ctx.SlotOf(mq.ServiceName).Value.(*mq.Queue)
	c := cap.NewClient()
	c.Bind(q, "provider", "test.add")
	x.got = c.Get("test.add")
	return nil
}
func (x *consumerPlugin) Start() error                          { return nil }
func (x *consumerPlugin) Run() error                            { return nil }
func (x *consumerPlugin) DealWithMessage(GoTenon.Message) error { return nil }
func (x *consumerPlugin) End() error                            { return nil }

// TestCapabilityDirectFetch 验证:消费者在装载期经消息向已装载的提供方请求能力,
// 同步拿到函数指针并可直接调用。
func TestCapabilityDirectFetch(t *testing.T) {
	root := GoTenon.New("root")
	m := GoTenon.NewManager(root)

	q := mq.New(64, 2)
	root.Isolate(mq.ServiceName)
	root.SlotOf(mq.ServiceName).Value = q
	q.Bind(m)

	if _, err := m.Register(mq.NewPlugin(), nil); err != nil {
		t.Fatal(err)
	}
	prov := &providerPlugin{}
	cons := &consumerPlugin{}
	if _, err := m.Register(prov, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Register(cons, nil); err != nil {
		t.Fatal(err)
	}

	if err := m.Enable("mq"); err != nil {
		t.Fatal(err)
	}
	if err := m.Enable("provider"); err != nil {
		t.Fatal(err)
	}
	if err := m.Enable("consumer"); err != nil {
		t.Fatal(err)
	}

	fn, ok := cons.got.(func(int, int) int)
	if !ok || fn == nil {
		t.Fatalf("未拿到能力函数指针: %T", cons.got)
	}
	if got := fn(2, 3); got != 5 {
		t.Fatalf("调用结果错: %d,期望 5", got)
	}
}
