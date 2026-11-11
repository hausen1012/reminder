// values 集中服务层对「任意 JSON 值」的规整函数。
//
// 通道配置与 AI 工具入参都来自前端或模型输出，类型不可信；读取时统一走这里。
package services

import "strings"

// asString 取字符串值；非字符串返回空串。
func asString(v any) string {
	s, _ := v.(string)
	return s
}

// asBool 取布尔值；非布尔返回 false。
func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

// asMap 取对象值；非对象返回 nil。
func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// asPositiveInt 取正整数值（兼容 JSON 的 float64）；非正数或无法识别返回 0。
func asPositiveInt(v any) int {
	switch n := v.(type) {
	case int:
		if n > 0 {
			return n
		}
	case int32:
		if n > 0 {
			return int(n)
		}
	case int64:
		if n > 0 {
			return int(n)
		}
	case float64:
		if n > 0 {
			return int(n)
		}
	}
	return 0
}

// asUintSlice 把 JSON 数字数组转成 []uint，非正数项被丢弃。
func asUintSlice(v any) []uint {
	if u, ok := v.([]uint); ok {
		return u
	}
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]uint, 0, len(items))
	for _, item := range items {
		switch n := item.(type) {
		case float64:
			if n > 0 {
				out = append(out, uint(n))
			}
		case int:
			if n > 0 {
				out = append(out, uint(n))
			}
		}
	}
	return out
}

// asStringSlice 把字符串数组（[]string 或 JSON 的 []any）转成去空白、去空项的列表。
func asStringSlice(v any) []string {
	switch items := v.(type) {
	case []string:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if s := strings.TrimSpace(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			s, _ := item.(string)
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
