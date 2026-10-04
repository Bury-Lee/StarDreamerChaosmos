// 这里放辅助结构体
package config

import (
	"sync"
	"sync/atomic"
)

// Config 是线程安全的只读配置快照容器:句柄稳定,内部快照原子替换。
type Config struct {
	snap atomic.Value // map[string]any:合并后的只读快照
	ver  atomic.Uint64
	mu   sync.Mutex // 串行化写入:同一时刻只允许一次装载
}

// New 创建一个空配置容器;宿主应在建 Manager 之前把它预挂到 root。
func New() *Config {
	c := &Config{}
	c.snap.Store(map[string]any{})
	return c
}

// store 原子替换整份快照并递增版本。
func (c *Config) store(m map[string]any) {
	c.snap.Store(m)
	c.ver.Add(1)
}

// Version 返回当前快照版本,每次成功装载 +1。
func (c *Config) Version() uint64 { return c.ver.Load() }

// Snapshot 返回提供给指定名字插件的快照(只读,调用方不得修改),为空时返回全部快照
func (c *Config) Snapshot(Name string) any {
	if Name == "" {
		return c.snapshot()
	} else {
		result := c.snapshot()
		return result[Name]
	}
}

// Section 返回某组件配置段的只读快照;不存在返回 nil。
// 供宿主/组件省去 Snapshot 的类型断言。
func (c *Config) Section(name string) map[string]any {
	sec, _ := c.Snapshot(name).(map[string]any)
	return sec
}

func (c *Config) snapshot() map[string]any {
	if m, ok := c.snap.Load().(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// Spec 是宿主装载本组件时传入的规格。
type Spec struct {
	Defaults  map[string]any // 内置默认(最低优先级)
	Path      string         // 配置文件路径(中优先级);由本组件读取解析,读不到则跳过
	File      map[string]any // 已解析的配置(中优先级,可选;与 Path 合并,File 在前)
	Required  []string       // 必填键(点分路径);缺失或空即报错
	EnvPrefix string         // 环境变量前缀,默认 "APP_"
}
