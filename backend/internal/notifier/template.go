// 模板渲染器：把 "{{name}}" 替换为 vars["name"]。
//
// 设计取舍：
//   - 不支持嵌套、函数调用、条件表达式；
//   - vars 中不存在的占位符原样保留（便于调试）；
//   - 不做递归展开（vars 的值里再含 {{x}} 也不会被二次替换）；
//   - {{ 与 }} 中允许空白与下划线，名称匹配 [A-Za-z_][A-Za-z0-9_.]*。
package notifier

import (
	"regexp"
	"strings"
)

var placeholderRe = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_.]*)\s*\}\}`)

// Render 渲染模板字符串，未在 vars 中找到的占位符原样保留。
func Render(tmpl string, vars map[string]string) string {
	if tmpl == "" {
		return ""
	}
	return placeholderRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		sub := placeholderRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		name := sub[1]
		if v, ok := vars[name]; ok {
			return v
		}
		return match
	})
}

// RenderMap 对一组键值都做渲染，返回新 map。
func RenderMap(in map[string]string, vars map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = Render(v, vars)
	}
	return out
}

// HasPlaceholder 用于在 UI 提示是否还有未填充的占位符。
func HasPlaceholder(s string) bool {
	return strings.Contains(s, "{{") && placeholderRe.MatchString(s)
}
