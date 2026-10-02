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

func (c *Config) snapshot() map[string]any {
	if m, ok := c.snap.Load().(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// Spec 是宿主装载本组件时传入的规格。
type Spec struct {
	Defaults  map[string]any // 内置默认(最低优先级)
	File      map[string]any // 配置文件解析结果(中优先级)
	Required  []string       // 必填键(点分路径);缺失或空即报错
	EnvPrefix string         // 环境变量前缀,默认 "APP_"
}
