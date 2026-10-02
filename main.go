// main.go 宿主入口,只负责启动内核并装配插件,业务都放在各文件夹里。
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"GoTenon"
	"StarDreamerChaosmos/core/config"
	"StarDreamerChaosmos/example/example"
	"StarDreamerChaosmos/example/watcher"
	"StarDreamerChaosmos/flag"
	"StarDreamerChaosmos/mq"
)

// pluginNames 是组件清单:注册、启动、描述展示的单一来源。
var pluginNames = []string{"config", "example", "flag", "mq", "watcher"}

func main() {
	// 1. 根上下文
	root := GoTenon.New("app")

	// 2. 宿主把原始命令行参数放进共享槽位(公共祖先),供 flag 插件解析
	root.Isolate(flag.ArgsService)
	root.SlotOf(flag.ArgsService).Value = os.Args[0:]

	// 3. 宿主预挂共享能力
	root.Isolate(config.ServiceName)
	root.SlotOf(config.ServiceName).Value = config.New()

	root.Isolate(flag.FlagsService)

	queue := mq.New(256, 4)
	root.Isolate(mq.ServiceName)
	root.SlotOf(mq.ServiceName).Value = queue

	// 4. 启动内核(中央管理器),并给队列绑定 Manager
	m := GoTenon.NewManager(root)
	queue.Bind(m)

	// 5. 注册插件
	if _, err := m.Register(config.NewPlugin(), config.Spec{
		Defaults: map[string]any{
			"example": map[string]any{"greeting": "hello"},
		},
	}); err != nil {
		panic(err)
	}
	if _, err := m.Register(example.NewPlugin(), nil); err != nil {
		panic(err)
	}
	if _, err := m.Register(flag.NewPlugin(), nil); err != nil {
		panic(err)
	}
	if _, err := m.Register(mq.NewPlugin(), nil); err != nil {
		panic(err)
	}
	if _, err := m.Register(watcher.NewPlugin(), nil); err != nil {
		panic(err)
	}

	// 6. 分层启动规范:内核 → bus(mq) → flag/config(支柱) → 业务组件

	// 6.1 先上 bus,并把内核生命周期事件桥接成队列主题
	if err := m.Enable("mq"); err != nil {
		panic(err)
	}
	stopBridge, err := m.Subscribe(GoTenon.Subscription{Subscriber: "mq"})
	if err != nil {
		panic(err)
	}

	// 6.2 再上 flag:Apply 解析指令,Start 执行前指令
	if err := m.Enable("flag"); err != nil {
		panic(err)
	}
	fp := flagPlugin(m)
	handleCommands(fp)

	// help / plugin details 是前功能:输出后即退出,不再启动其它组件
	if fp != nil && fp.Help() {
		stopBridge()
		queue.Shutdown()
		return
	}
	if fp != nil && fp.PluginDetails() {
		showPlugins(m, pluginNames)
		stopBridge()
		queue.Shutdown()
		return
	}

	// 6.3 再上 config(第二根支柱)
	if err := m.Enable("config"); err != nil {
		panic(err)
	}

	// 7. 最后上业务组件(逐个 Enable = 手动控制启动顺序)
	for _, name := range []string{"watcher", "example"} {
		if err := m.Enable(name); err != nil {
			panic(err)
		}
	}

	// 8. 运行期:向插件发消息(内核原封投递)
	_ = m.Send(GoTenon.Message{Name: "example", Type: GoTenon.TypeRaw, Data: "StarDreamer"})

	// 通过内核消息发布到消息队列(TypeMQPublish)→ 入队 → 后台消费者投递给订阅者
	if err := queue.Publish(watcher.TopicPing, map[string]any{"from": "host"}); err != nil {
		fmt.Printf("[宿主] publish 失败: %v\n", err)
	}

	if rt, ok := m.Get("example"); ok {
		if st := rt.Plugin.Status(); st != nil {
			fmt.Printf("[宿主] example 实时状态: %v\n", *st)
		}
	}
	if rt, ok := m.Get("mq"); ok {
		if st := rt.Plugin.Status(); st != nil {
			fmt.Printf("[宿主] mq 状态: %v\n", *st)
		}
	}

	// 队列是异步的:等一小会儿让待投递消息排空,再停机
	time.Sleep(200 * time.Millisecond)

	// 9. 停机:按依赖者先走的顺序卸载,逆序回收副作用
	stopBridge()
	for _, name := range []string{"example", "config", "watcher", "mq", "flag"} {
		if _, ok := m.Get(name); ok {
			if err := m.Disable(name); err != nil {
				panic(err)
			}
		}
	}
	queue.Shutdown()
}

// flagPlugin 取 flag 组件实例(用于读取解析结果)。
func flagPlugin(m *GoTenon.Manager) *flag.Plugin {
	rt, ok := m.Get("flag")
	if !ok {
		return nil
	}
	fp, _ := rt.Plugin.(*flag.Plugin)
	return fp
}

// handleCommands 回显 flag 解析出的指令。
func handleCommands(fp *flag.Plugin) {
	if fp == nil {
		return
	}
	if all, blog, ok := fp.InitDB(); ok {
		fmt.Printf("[宿主] 收到 initdb 指令 (all=%v blog=%v):待数据库组件就绪后执行\n", all, blog)
	}
	if s, ok := fp.Setting(); ok {
		fmt.Printf("[宿主] 收到 setting 指令:init=%q copy=%q type=%s\n", s.InitSetting, s.CopySetting, s.Type)
	}
	if fp.ShouldRun() {
		fmt.Println("[宿主] 收到 run 指令:进入常驻")
	}
}

// showPlugins 打印各组件描述(DESC)。
func showPlugins(m *GoTenon.Manager, names []string) {
	fmt.Println("[宿主] 组件描述(DESC):")
	for _, name := range names {
		rt, ok := m.Get(name)
		if !ok {
			continue
		}
		desc := rt.Plugin.Desc()
		parts := make([]string, 0, len(desc))
		for k, v := range desc {
			parts = append(parts, k+"="+v)
		}
		sort.Strings(parts)
		fmt.Printf("  %-10s %s\n", name, strings.Join(parts, "  "))
	}
}
