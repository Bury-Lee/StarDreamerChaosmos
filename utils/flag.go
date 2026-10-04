package utils

import (
	"fmt"
	"strings"
)

//这里放一些方便的解析函数

// IsFlag 报告 token 是否是一个选项(以 '-' 开头)。
func IsFlag(arg string) bool {
	return len(arg) > 0 && arg[0] == '-'
}

// SplitFlag 把一个选项 token 拆成 name / value:
//
//	"-name"        → ("name", "", false)
//	"--name=value" → ("name", "value", true)
//	"run" / ""     → ("", "", false)   非选项
func SplitFlag(arg string) (name, value string, hasValue bool) {
	if !IsFlag(arg) {
		return "", "", false
	}
	body := strings.TrimLeft(arg, "-")
	if body == "" {
		return "", "", false
	}
	if i := strings.IndexByte(body, '='); i >= 0 {
		return body[:i], body[i+1:], true
	}
	return body, "", false
}

// NextValue 取选项的取值:优先用内联值(hasInline),否则消费下一个 token(args[i+1])。
// 返回取值与消费到的索引(调用方应把 i 更新为返回的 next);缺值返回错误。
func NextValue(args []string, i int, name, inline string, hasInline bool) (value string, next int, err error) {
	if hasInline {
		return inline, i, nil
	}
	if i+1 >= len(args) {
		return "", i, fmt.Errorf("选项 %s 缺少取值", name)
	}
	return args[i+1], i + 1, nil
}

// ConfigPath 从 argv(含程序名)里取 -config/--config 指定的配置文件路径;未指定返回 def。
func ConfigPath(argv []string, def string) string {
	args := argv
	if len(args) > 0 {
		args = args[1:] // 跳过程序名
	}
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
	return def
}

// SplitList 把逗号分隔的字符串拆成去空白的列表。
func SplitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
