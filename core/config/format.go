// format.go —— 配置的默认模板与序列化(YAML / JSON),供 init-setting / copy-setting 使用。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"StarDreamerChaosmos/flag"
)

// defaultSetting 是 init-setting 输出的内置默认配置模板。
func defaultSetting() map[string]any {
	return map[string]any{
		"system": map[string]any{
			"name": "StarDreamerChaosmos",
			"addr": "127.0.0.1",
			"port": 8080,
			"env":  "dev",
		},
		"log": map[string]any{
			"level": "info",
			"dir":   "./logs",
		},
		"database": map[string]any{
			"driver": "sqlite",
			"path":   "./data/app.db",
		},
		"components": map[string]any{
			"mq":     true,
			"flag":   true,
			"config": true,
		},
	}
}

// encodeSetting 按格式序列化配置。
func encodeSetting(v any, t flag.Type) ([]byte, error) {
	if t == flag.Json {
		return json.MarshalIndent(v, "", "  ")
	}
	return encodeYAML(v), nil
}

// writeSetting 把数据写到路径(自动建父目录)。
func writeSetting(path string, data []byte) error {
	if path == "" {
		return fmt.Errorf("config: 输出路径为空")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

// ---------- 极简 YAML 输出(仅覆盖模板所需的嵌套 map / list / 标量) ----------

func encodeYAML(v any) []byte {
	var b strings.Builder
	writeYAML(&b, v, 0)
	return []byte(b.String())
}

func writeYAML(b *strings.Builder, v any, indent int) {
	pad := strings.Repeat("  ", indent)
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			val := t[k]
			switch child := val.(type) {
			case map[string]any:
				fmt.Fprintf(b, "%s%s:\n", pad, k)
				writeYAML(b, child, indent+1)
			case []any:
				fmt.Fprintf(b, "%s%s:\n", pad, k)
				for _, item := range child {
					fmt.Fprintf(b, "%s  - %s\n", pad, scalar(item))
				}
			default:
				fmt.Fprintf(b, "%s%s: %s\n", pad, k, scalar(val))
			}
		}
	case []any:
		for _, item := range t {
			fmt.Fprintf(b, "%s- %s\n", pad, scalar(item))
		}
	default:
		fmt.Fprintf(b, "%s%s\n", pad, scalar(v))
	}
}

func scalar(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", x)
	}
}
