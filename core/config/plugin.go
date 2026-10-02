// Package config 是配置读取组件。
//
// 职责:把「内置默认 < 配置文件解析结果 < 环境变量」按优先级合并成一份只读快照,
// 写入宿主预挂在 root 的共享容器(服务名 svc/config);其它组件 Inject("config")
// 后经 ctx.SlotOf("svc/config") 取得同一个 *Config,再用 utils 按路径取值。
//
// 文件约定:
//   - plugin.go  组件本体(GoTenon 契约与生命周期)
//   - types.go   辅助结构体(Config 容器、Spec)
//   - read.go    功能封装(合并、环境变量、归一、校验)
//   - format.go  功能封装(默认模板、序列化、写文件)
package config

import (
	"fmt"
	"os"
	"sync"

	"GoTenon"
	"StarDreamerChaosmos/flag"
)

// ServiceName 是配置容器在上下文里的服务名(宿主预挂到 root)。
const ServiceName = "svc/config"

// Plugin 是配置读取组件本体,实现 GoTenon.PluginInfo。
type Plugin struct {
	mu     sync.Mutex
	holder *Config
}

// NewPlugin 创建配置组件。
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "config" }
func (p *Plugin) Desc() map[string]string {
	return map[string]string{"provides": ServiceName, "note": "配置读取:默认<文件<环境变量"}
}
func (p *Plugin) Inject() []string { return nil }

func (p *Plugin) Status() *map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.holder == nil {
		return statusOf("pending", 0, 0)
	}
	return statusOf("ready", p.holder.Version(), len(p.holder.snapshot()))
}

// Register 在写入插件表之前调用,本组件无需额外登记。
func (p *Plugin) Register() error { return nil }

// Apply 合并配置并写入共享容器;期间注册的 Disposer 卸载时逆序回收。
func (p *Plugin) Apply(ctx *GoTenon.GoTenonContext, cfg any) error {
	slot := ctx.SlotOf(ServiceName)
	if slot == nil || slot.Value == nil {
		return fmt.Errorf("config: 宿主未预挂 %s", ServiceName)
	}
	holder, ok := slot.Value.(*Config)
	if !ok {
		return fmt.Errorf("config: %s 类型不符: %T", ServiceName, slot.Value)
	}

	spec, _ := cfg.(Spec)
	merged, err := loadSnapshot(spec)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	p.mu.Lock()
	p.holder = holder
	p.mu.Unlock()

	holder.store(merged)
	fmt.Printf("[config] 装载完成:v%d,顶层键 %d 个\n", holder.Version(), len(merged))

	ctx.Register(func() error {
		p.mu.Lock()
		p.holder = nil
		p.mu.Unlock()
		holder.store(map[string]any{}) // 卸载后清空,消费者读到空快照
		return nil
	})
	return nil
}

func (p *Plugin) Start() error { return nil }
func (p *Plugin) Run() error   { return nil }
func (p *Plugin) End() error   { return nil }

func (p *Plugin) DealWithMessage(m GoTenon.Message) error {
	switch m.Type {
	case GoTenon.TypeIndex:
		if ev, ok := m.Data.(GoTenon.IndexEvent); ok {
			fmt.Printf("[config] 索引事件: %-6s %s\n", ev.Kind, ev.Plugin)
		}
	case flag.TypeSettingRequest:
		s, ok := m.Data.(flag.Setting)
		if !ok {
			return fmt.Errorf("config: TypeSettingRequest 载荷类型不符: %T", m.Data)
		}
		return p.applySetting(s)
	}
	return nil
}

// applySetting 执行配置初始化 / 拷贝(由 flag 经 bus 转发而来)。
func (p *Plugin) applySetting(s flag.Setting) error {
	switch {
	case s.InitSetting != "":
		data, err := encodeSetting(defaultSetting(), s.Type)
		if err != nil {
			return fmt.Errorf("config: 序列化默认配置失败: %w", err)
		}
		if err := writeSetting(s.InitSetting, data); err != nil {
			return fmt.Errorf("config: 写入默认配置失败: %w", err)
		}
		fmt.Printf("[config] 已输出默认配置 → %s (%s)\n", s.InitSetting, s.Type)

	case s.CopySetting != "":
		src := flag.DefaultSettingPath
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("config: 读取现有配置 %s 失败: %w", src, err)
		}
		if err := writeSetting(s.CopySetting, data); err != nil {
			return fmt.Errorf("config: 拷贝配置失败: %w", err)
		}
		fmt.Printf("[config] 已拷贝配置 %s → %s\n", src, s.CopySetting)

	default:
		return fmt.Errorf("config: Setting 指令为空")
	}
	return nil
}

// Function 声明对外能力(MCP 风格描述),供内核索引与发现。
func (p *Plugin) Function() map[string]any {
	return map[string]any{"config.get": map[string]any{"description": "按点分路径读取配置"}}
}
func (p *Plugin) ExecuteFunction(any) {}

func statusOf(state string, version uint64, keys int) *map[string]any {
	m := map[string]any{"state": state, "version": version, "keys": keys}
	return &m
}
