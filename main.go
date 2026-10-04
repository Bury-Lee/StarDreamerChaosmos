// main.go 宿主入口:只负责启动内核并装配插件。
package main

import (
	"fmt"
	"time"

	"GoTenon"
	"StarDreamerChaosmos/common"
	"StarDreamerChaosmos/core/config"
	"StarDreamerChaosmos/dialer"
	"StarDreamerChaosmos/gateway"
	"StarDreamerChaosmos/mq"
	"StarDreamerChaosmos/utils"
)

func main() {
	// 1. 根上下文
	root := GoTenon.New("app")

	// 2. 宿主预挂"共享"能力:凡是会被别的组件(含业务组件)读取的,才上挂到 root。
	//    仅本组件自用的槽位,由组件在自己的 Apply 里 Isolate,不在此声明。
	cfg := config.New()
	root.Isolate(config.ServiceName)
	root.SlotOf(config.ServiceName).Value = cfg

	queue := mq.New(256, 4)
	root.Isolate(mq.ServiceName)
	root.SlotOf(mq.ServiceName).Value = queue

	d := dialer.New()
	root.Isolate(dialer.ServiceName)
	root.SlotOf(dialer.ServiceName).Value = d

	router := gateway.NewRouter()
	root.Isolate(gateway.ServiceName)
	root.SlotOf(gateway.ServiceName).Value = router

	// 3. 启动内核
	m := GoTenon.NewManager(root)
	queue.Bind(m)

	// 4. 注册全部组件(停用的也注册:插件详情可见,且可随时由配置开启)
	for _, c := range components {
		if _, err := m.Register(c.plugin, c.cfg); err != nil {
			panic(err)
		}
	}

	// 5. 分层启动
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
	if fp == nil {
		return
	}
	// 前功能(help/run/initdb/setting)已在 flag.Start 内输出;help / plugin details 输出后即退出。
	// plugin details 需经 Manager 读活体描述,只能由宿主执行——flag 装载期回调 Manager 会死锁。
	if fp.Exit() {
		if fp.PluginDetails() {
			showPlugins(m, components)
		}
		stopBridge()
		queue.Shutdown()
		return
	}

	// 启动选项:是否要求初始化数据库(默认否)。
	// 组件在上线(Apply)阶段读该槽位,据此决定是否执行模型迁移——平时启动不建表。
	boot := common.Boot{}
	if _, _, ok := fp.InitDB(); ok {
		boot.InitDB = true
	}
	root.Isolate(common.BootService)
	root.SlotOf(common.BootService).Value = boot

	// 6. 先启用 config,拿到合并后的配置快照(内置默认 < 配置文件 < 环境变量)
	if err := m.Enable("config"); err != nil {
		panic(err)
	}
	started := []string{"mq", "flag", "config"}

	// 7. 启用核心骨架的其余部分(dialer / gateway):固定启用,配置不可关闭
	for _, c := range components {
		if !c.core || utils.Contains(started, c.name) {
			continue
		}
		if err := m.Enable(c.name); err != nil {
			panic(err)
		}
		started = append(started, c.name)
	}

	// 8. 启用业务组件:读各组件配置段的 enable 决定开关(缺省启用)
	for _, c := range components {
		if c.core {
			continue
		}
		if !utils.Bool(cfg.Section(c.name), "enable", true) {
			fmt.Printf("[宿主] 组件 %s 已按配置停用\n", c.name)
			continue
		}
		if err := m.Enable(c.name); err != nil {
			panic(err)
		}
		started = append(started, c.name)
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

	// 9. 运行:run 指令 → 常驻;否则跑一次自测
	if fp.ShouldRun() {
		fmt.Println("[宿主] 常驻启动,等待中断(Ctrl+C)...")
		utils.WaitSignal()
	} else {
		selfTest("http://" + utils.Str(cfg.Section("gateway"), "addr", "127.0.0.1:18080"))
		time.Sleep(200 * time.Millisecond)
	}

	// 10. 停机(依赖者先走):逆序卸载本次启用的组件(触发 down 让网关摘路由),再退订
	for i := len(started) - 1; i >= 0; i-- {
		name := started[i]
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
