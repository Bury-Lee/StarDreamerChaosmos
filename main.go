// main.go 宿主入口:只负责启动内核并装配插件。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"GoTenon"
	"StarDreamerChaosmos/core/config"
	"StarDreamerChaosmos/dialer"
	"StarDreamerChaosmos/flag"
	"StarDreamerChaosmos/gateway"
	"StarDreamerChaosmos/mq"
	"StarDreamerChaosmos/services/user"
)

// pluginNames 是组件清单:注册、启动、描述展示的单一来源。
var pluginNames = []string{"config", "dialer", "flag", "gateway", "mq", "user"}

func main() {
	// 1. 根上下文
	root := GoTenon.New("app")

	// 2. 共享槽位:原始参数
	root.Isolate(flag.ArgsService)
	root.SlotOf(flag.ArgsService).Value = os.Args[0:]
	root.Isolate(flag.FlagsService)

	// 3. 宿主预挂共享能力
	root.Isolate(config.ServiceName)
	root.SlotOf(config.ServiceName).Value = config.New()

	queue := mq.New(256, 4)
	root.Isolate(mq.ServiceName)
	root.SlotOf(mq.ServiceName).Value = queue

	d := dialer.New()
	root.Isolate(dialer.ServiceName)
	root.SlotOf(dialer.ServiceName).Value = d

	router := gateway.NewRouter()
	root.Isolate(gateway.ServiceName)
	root.SlotOf(gateway.ServiceName).Value = router

	// 4. 启动内核
	m := GoTenon.NewManager(root)
	queue.Bind(m)

	// 5. 注册插件(config 的 File 来自配置文件)
	if _, err := m.Register(config.NewPlugin(), config.Spec{
		Defaults: map[string]any{
			"gateway": map[string]any{"addr": "127.0.0.1:18080", "engine": "gin"},
			"user":    map[string]any{"db": "data/sdc.db"},
		},
		File: loadSetting(configPath()),
	}); err != nil {
		panic(err)
	}
	if _, err := m.Register(flag.NewPlugin(), nil); err != nil {
		panic(err)
	}
	if _, err := m.Register(mq.NewPlugin(), nil); err != nil {
		panic(err)
	}
	if _, err := m.Register(dialer.NewPlugin(), nil); err != nil {
		panic(err)
	}
	if _, err := m.Register(gateway.NewPlugin(), nil); err != nil {
		panic(err)
	}
	if _, err := m.Register(user.NewPlugin(), nil); err != nil {
		panic(err)
	}

	// 6. 分层启动
	if err := m.Enable("mq"); err != nil {
		panic(err)
	}
	stopBridge, err := m.Subscribe(GoTenon.Subscription{Subscriber: "mq"})
	if err != nil {
		panic(err)
	}

	if err := m.Enable("flag"); err != nil {
		panic(err)
	}
	fp := flagPlugin(m)
	handleCommands(fp)
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

	for _, name := range []string{"config", "dialer", "gateway", "user"} {
		if err := m.Enable(name); err != nil {
			panic(err)
		}
	}
	// 所有提供方注册完服务后,再启动拨号器的本地 gRPC server
	d.Start()
	// 网关:先落地已入队的路由消息,再开始接收请求
	if rt, ok := m.Get("gateway"); ok {
		if gp, ok := rt.Plugin.(*gateway.Plugin); ok {
			gp.StartServing()
		}
	}

	// 网关订阅内核事件:组件下线时自动摘除其路由
	stopGateway, err := m.Subscribe(GoTenon.Subscription{Subscriber: "gateway"})
	if err != nil {
		panic(err)
	}

	// 7. 运行:run 指令 → 常驻;否则跑一次自测
	if fp != nil && fp.ShouldRun() {
		fmt.Println("[宿主] 常驻启动,等待中断(Ctrl+C)...")
		waitSignal()
	} else {
		selfTest("http://127.0.0.1:18080")
		time.Sleep(200 * time.Millisecond)
	}

	// 8. 停机(依赖者先走):先卸载组件(触发 down 让网关摘路由),再退订
	for _, name := range []string{"user", "gateway", "dialer", "config", "mq", "flag"} {
		if _, ok := m.Get(name); ok {
			if err := m.Disable(name); err != nil {
				panic(err)
			}
		}
	}
	stopGateway()
	stopBridge()
	_ = d.Close()
	queue.Shutdown()
}

// configPath 从 os.Args 里取配置文件路径,缺省 Setting.yaml。
func configPath() string {
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-config" || a == "--config":
			if i+1 < len(args) {
				return args[i+1]
			}
		case strings.HasPrefix(a, "-config="):
			return strings.TrimPrefix(a, "-config=")
		case strings.HasPrefix(a, "--config="):
			return strings.TrimPrefix(a, "--config=")
		}
	}
	return "Setting.yaml"
}

// loadSetting 读取 YAML 配置文件;读不到则返回 nil(用内置默认)。
func loadSetting(path string) map[string]any {
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("[宿主] 未读到配置文件 %s(%v),使用内置默认\n", path, err)
		return nil
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		fmt.Printf("[宿主] 解析配置文件 %s 失败: %v\n", path, err)
		return nil
	}
	fmt.Printf("[宿主] 已读配置文件 %s\n", path)
	return m
}

// waitSignal 阻塞直到收到中断信号。
func waitSignal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
}

func flagPlugin(m *GoTenon.Manager) *flag.Plugin {
	rt, ok := m.Get("flag")
	if !ok {
		return nil
	}
	fp, _ := rt.Plugin.(*flag.Plugin)
	return fp
}

func handleCommands(fp *flag.Plugin) {
	if fp == nil {
		return
	}
	if s, ok := fp.Setting(); ok {
		fmt.Printf("[宿主] 收到 setting 指令:init=%q copy=%q type=%s\n", s.InitSetting, s.CopySetting, s.Type)
	}
	if fp.ShouldRun() {
		fmt.Println("[宿主] 收到 run 指令:进入常驻")
	}
}

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

// selfTest 用 HTTP 打一遍全链路。
func selfTest(base string) {
	fmt.Println("== 全链路自测 ==")
	post := func(path, body string) (int, string) {
		resp, err := http.Post(base+path, "application/json", strings.NewReader(body))
		if err != nil {
			return 0, err.Error()
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, strings.TrimSpace(string(b))
	}

	code, body := post("/api/user/register", `{"username":"alice","password":"secret1"}`)
	fmt.Printf("  register alice        -> %d %s\n", code, body)

	code, body = post("/api/user/register", `{"username":"alice","password":"secret1"}`)
	fmt.Printf("  register alice again  -> %d %s\n", code, body)

	code, body = post("/api/user/login", `{"username":"alice","password":"secret1"}`)
	fmt.Printf("  login alice           -> %d %s\n", code, body)
	token := jsonToken(body)

	code, body = post("/api/user/logout", `{"token":"`+token+`"}`)
	fmt.Printf("  logout                -> %d %s\n", code, body)

	code, body = post("/api/user/logout", `{"token":"`+token+`"}`)
	fmt.Printf("  logout again          -> %d %s\n", code, body)

	code, body = post("/api/user/login", `{"username":"alice","password":"wrong"}`)
	fmt.Printf("  login wrong password  -> %d %s\n", code, body)
}

func jsonToken(body string) string {
	var r struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	_ = json.Unmarshal([]byte(body), &r)
	return r.Data.Token
}
