package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"GoTenon"
	"StarDreamerChaosmos/core/config"
	"StarDreamerChaosmos/dialer"
	"StarDreamerChaosmos/flag"
	"StarDreamerChaosmos/gateway"
	"StarDreamerChaosmos/mq"
	"StarDreamerChaosmos/services/user"
	"StarDreamerChaosmos/utils"
)

// component 是一项组件登记。
//   - core = true:核心骨架,固定启用,配置不可关闭(缺了系统不成立);
//   - core = false:业务组件,由本组件配置段的 enable 决定开关(缺省启用)。
type component struct {
	name   string
	plugin GoTenon.PluginInfo
	cfg    any
	core   bool
}

// components 是组件登记表:注册、启用、展示的单一来源;新增组件只加一行。
var components = []component{
	{name: "mq", plugin: mq.NewPlugin(), core: true},
	{name: "flag", plugin: flag.NewPlugin(), cfg: os.Args[0:], core: true},
	{name: "config", plugin: config.NewPlugin(), core: true, cfg: config.Spec{
		Defaults: map[string]any{
			"gateway": map[string]any{"addr": "127.0.0.1:18080", "engine": "gin"},
			"user":    map[string]any{"enable": true, "db": "data/sdc.db"},
		},
		Path: utils.ConfigPath(os.Args, "Setting.yaml"),
	}},
	{name: "dialer", plugin: dialer.NewPlugin(), core: true},
	{name: "gateway", plugin: gateway.NewPlugin(), core: true},
	{name: "user", plugin: user.NewPlugin()},
}

func flagPlugin(m *GoTenon.Manager) *flag.Plugin {
	rt, ok := m.Get("flag")
	if !ok {
		return nil
	}
	fp, _ := rt.Plugin.(*flag.Plugin)
	return fp
}

func showPlugins(m *GoTenon.Manager, comps []component) {
	fmt.Println("[宿主] 组件描述(DESC):")
	for _, c := range comps {
		rt, ok := m.Get(c.name)
		if !ok {
			continue
		}
		desc := rt.Plugin.Desc()
		parts := make([]string, 0, len(desc))
		for k, v := range desc {
			parts = append(parts, k+"="+v)
		}
		sort.Strings(parts)
		role := "业务"
		if c.core {
			role = "核心"
		}
		fmt.Printf("  %-10s [%s] %s\n", c.name, role, strings.Join(parts, "  "))
	}
}

// selfTest 用 HTTP 启动一遍全链路。
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
	token := utils.JsonToken(body)

	code, body = post("/api/user/logout", `{"token":"`+token+`"}`)
	fmt.Printf("  logout                -> %d %s\n", code, body)

	code, body = post("/api/user/logout", `{"token":"`+token+`"}`)
	fmt.Printf("  logout again          -> %d %s\n", code, body)

	code, body = post("/api/user/login", `{"username":"alice","password":"wrong"}`)
	fmt.Printf("  login wrong password  -> %d %s\n", code, body)
}
