// Package utils 收纳简单、无状态的纯函数(输入 → 输出,无副作用)。
package utils

import "strings"

// Lookup 按点分路径在嵌套 map 中取值,如 "database.host"。
// 路径与键均按小写匹配(配置装载时已把键归一为小写)。
func Lookup(m map[string]any, path string) (any, bool) {
	var cur any = m
	for _, seg := range strings.Split(strings.ToLower(path), ".") {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = mm[seg]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// Str 按路径取字符串;缺失/空串/非字符串 → def。
func Str(m map[string]any, path, def string) string {
	if v, ok := Lookup(m, path); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

// Int 按路径取整数(yaml 可能解析为 int/int64/float64);缺失/非数值 → def。
func Int(m map[string]any, path string, def int) int {
	v, ok := Lookup(m, path)
	if !ok {
		return def
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return def
	}
}

// Float 按路径取浮点(int/int64/float64);缺失/非数值 → def。
func Float(m map[string]any, path string, def float64) float64 {
	v, ok := Lookup(m, path)
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return def
	}
}

// Bool 按路径取布尔;缺失/非布尔 → def。
func Bool(m map[string]any, path string, def bool) bool {
	if v, ok := Lookup(m, path); ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}
