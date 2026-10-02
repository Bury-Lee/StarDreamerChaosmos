package config

import (
	"fmt"
	"os"
	"strings"

	"StarDreamerChaosmos/utils"
)

// 环境变量默认前缀:APP_DATABASE__HOST → database.host("__" 作层级分隔)。
const defaultEnvPrefix = "APP_"

// loadSnapshot 按「默认 < 文件 < 环境变量」合并并校验,产出一份只读快照。
func loadSnapshot(spec Spec) (map[string]any, error) {
	merged := mergeMaps(spec.Defaults, spec.File)
	applyEnv(merged, spec.EnvPrefix)
	merged = normalize(merged)

	if missing := missingKeys(merged, spec.Required); len(missing) > 0 {
		return nil, fmt.Errorf("缺少必填项 %v", missing)
	}
	return merged, nil
}

// mergeMaps 按层深合并,后者覆盖前者。
func mergeMaps(layers ...map[string]any) map[string]any {
	out := map[string]any{}
	for _, m := range layers {
		mergeInto(out, m)
	}
	return out
}

func mergeInto(dst, src map[string]any) {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				mergeInto(dm, sm)
				continue
			}
			nd := map[string]any{}
			mergeInto(nd, sm)
			dst[k] = nd
			continue
		}
		dst[k] = v
	}
}

// applyEnv 把带前缀的环境变量写入配置树,键路径用 "__" 分隔。
// 例:APP_DATABASE__HOST=db → database.host = "db"。
func applyEnv(m map[string]any, prefix string) {
	if prefix == "" {
		prefix = defaultEnvPrefix
	}
	for _, e := range os.Environ() {
		k, v, ok := strings.Cut(e, "=")
		if !ok || !strings.HasPrefix(k, prefix) {
			continue
		}
		k = strings.TrimPrefix(k, prefix)
		if k == "" {
			continue
		}
		setPath(m, strings.ToLower(strings.ReplaceAll(k, "__", ".")), v)
	}
}

func setPath(m map[string]any, path, val string) {
	parts := strings.Split(path, ".")
	cur := m
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = val
			return
		}
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
}

// normalize 递归把键归一为小写,避免大小写造成两套语义。
func normalize(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if sub, ok := v.(map[string]any); ok {
			v = normalize(sub)
		}
		out[strings.ToLower(k)] = v
	}
	return out
}

// missingKeys 返回缺失或为空的必填键。
func missingKeys(m map[string]any, required []string) []string {
	var missing []string
	for _, path := range required {
		v, ok := utils.Lookup(m, path)
		if !ok || v == nil || v == "" {
			missing = append(missing, path)
		}
	}
	return missing
}
