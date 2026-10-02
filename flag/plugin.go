// Package flag 是命令行解析插件:
// 宿主把 os.Args 放进共享槽位;Apply 阶段解析并登记后功能待办(需要 ctx 拿槽位),
// Start 阶段执行不依赖服务的前功能;后功能靠 bus 等待目标组件上线后再转发执行。
//
// 文件约定:
//   - plugin.go 组件本体(GoTenon 契约与生命周期)
//   - parse.go  功能封装(命令行解析)
//   - args.go   辅助结构体(Flags / initDB / Type / Setting / waitTask)
//   - enter.go  常量与硬编码
package flag

import (
	"fmt"
	"sync"

	"GoTenon"
	"StarDreamerChaosmos/mq"
)

// Plugin 是命令行解析插件,实现 GoTenon.PluginInfo。
type Plugin struct {
	mu    sync.Mutex
	out   *Flags
	queue *mq.Queue
	waits []*waitTask
}

// NewPlugin 创建命令行解析插件。
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "flag" }
func (p *Plugin) Desc() map[string]string {
	return map[string]string{"provides": FlagsService, "功能": "进行命令行解析"}
}
func (p *Plugin) Inject() []string { return nil }

// Status 返回 nil:本插件不向索引上报状态。
func (p *Plugin) Status() *map[string]any { return nil }

// Register 在写入插件表之前调用(此时还没有 ctx,拿不到参数槽位)。
func (p *Plugin) Register() error { return nil }

// Apply 解析指令,并接入 bus:订阅"组件上线",为后功能登记待办。
func (p *Plugin) Apply(ctx *GoTenon.GoTenonContext, _ any) error {
	// 1. 取宿主存入的原始参数,并做类型断言
	slot := ctx.SlotOf(ArgsService)
	if slot == nil || slot.Value == nil {
		return fmt.Errorf("flag: 宿主未预挂 %s", ArgsService)
	}
	raw, ok := slot.Value.([]string)
	if !ok {
		return fmt.Errorf("flag: %s 期望 []string,得到 %T", ArgsService, slot.Value)
	}

	// 2. 解析
	out, err := Parse(raw)
	if err != nil {
		return fmt.Errorf("flag: %w", err)
	}
	p.mu.Lock()
	p.out = out
	p.mu.Unlock()

	// 3. 发布结果到宿主预挂的共享槽位
	if dst := ctx.SlotOf(FlagsService); dst != nil {
		dst.Value = out
	}

	// 4. 接入消息队列:订阅组件上线;后功能登记待办(等目标组件上线再转发)
	if s := ctx.SlotOf(mq.ServiceName); s != nil && s.Value != nil {
		if q, ok := s.Value.(*mq.Queue); ok {
			p.mu.Lock()
			p.queue = q
			p.mu.Unlock()
			ctx.Register(q.Subscribe(mq.TopicComponentReady, "flag"))
			if out.Setting != nil {
				setting := *out.Setting
				p.addWait(mq.TopicComponentReady, "config", func() {
					_ = q.Forward("config", TypeSettingRequest, setting)
				})
			}
		}
	}

	// 5. 可逆副作用:卸载时清空
	ctx.Register(func() error {
		p.mu.Lock()
		p.out = nil
		p.queue = nil
		p.waits = nil
		p.mu.Unlock()
		return nil
	})
	fmt.Println("[flag] 指令解析完成")
	return nil
}

// Start 在 Apply 之后执行「前功能」——不依赖其它服务即可完成的动作。
// 「后功能」(如配置输出、初始化数据库)依赖的目标组件此时未必就绪,
// 由 Apply 登记的待办在目标组件上线后触发。
func (p *Plugin) Start() error {
	out := p.snapshot()
	if out == nil {
		return nil
	}
	if out.help {
		fmt.Println(Usage)
	}
	if out.run {
		fmt.Println("[flag] 执行指令 run:启动服务")
	}
	if out.InitDB != nil {
		switch {
		case out.InitDB.All:
			fmt.Println("[flag] 执行指令 initdb all:初始化全部数据库")
		case out.InitDB.Blog:
			fmt.Println("[flag] 执行指令 initdb blog:初始化博客库")
		}
	}
	return nil
}

func (p *Plugin) Run() error { return nil }
func (p *Plugin) End() error { return nil }

// DealWithMessage 处理内核索引事件与 bus 事件。
func (p *Plugin) DealWithMessage(m GoTenon.Message) error {
	switch m.Type {
	case GoTenon.TypeIndex:
		if ev, ok := m.Data.(GoTenon.IndexEvent); ok {
			fmt.Printf("[flag] 索引事件: %-6s %s\n", ev.Kind, ev.Plugin)
		}
	case mq.TypeMQEvent:
		ev, ok := m.Data.(mq.Event)
		if !ok {
			return nil
		}
		if ev.Topic == mq.TopicComponentReady {
			if idx, ok := ev.Data.(GoTenon.IndexEvent); ok && idx.Kind == "up" {
				p.fire(idx.Plugin)
			}
		}
	}
	return nil
}

func (p *Plugin) Function() map[string]any {
	return map[string]any{"flag.get": map[string]any{"description": "读取命令行解析结果"}}
}
func (p *Plugin) ExecuteFunction(any) {}

// ---------- 待办 ----------

func (p *Plugin) addWait(topic, component string, run func()) {
	p.mu.Lock()
	p.waits = append(p.waits, &waitTask{topic: topic, component: component, run: run})
	p.mu.Unlock()
}

// fire 触发并移除所有等待 component 上线后的待办。
func (p *Plugin) fire(component string) {
	p.mu.Lock()
	var remaining []*waitTask
	var todo []func()
	for _, w := range p.waits {
		if w.component == component {
			todo = append(todo, w.run)
		} else {
			remaining = append(remaining, w)
		}
	}
	p.waits = remaining
	p.mu.Unlock()
	for _, fn := range todo {
		fn()
	}
}

// ---------- 供宿主读取解析结果(字段未导出,故提供只读访问器) ----------

// ShouldRun 报告是否请求启动服务。
func (p *Plugin) ShouldRun() bool {
	out := p.snapshot()
	return out != nil && out.run
}

// Help 报告是否请求显示帮助。
func (p *Plugin) Help() bool {
	out := p.snapshot()
	return out != nil && out.help
}

// PluginDetails 报告是否请求显示各组件描述。
func (p *Plugin) PluginDetails() bool {
	out := p.snapshot()
	return out != nil && out.pluginDetails
}

// InitDB 返回数据库初始化指令;ok=false 表示没有该指令。
func (p *Plugin) InitDB() (all, blog, ok bool) {
	out := p.snapshot()
	if out == nil || out.InitDB == nil {
		return false, false, false
	}
	return out.InitDB.All, out.InitDB.Blog, true
}

// Setting 返回配置初始化 / 拷贝指令;ok=false 表示没有该指令。
func (p *Plugin) Setting() (*Setting, bool) {
	out := p.snapshot()
	if out == nil || out.Setting == nil {
		return nil, false
	}
	return out.Setting, true
}

func (p *Plugin) snapshot() *Flags {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.out
}
