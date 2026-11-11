// 日志辅助：header 只记名字不记取值（Authorization 等本身就是凭据）。
package notifier

import "sort"

// headerKeyList 返回 map 形式的 header 名列表，按字母序排列；只记名字不记值。
func headerKeyList(h map[string]string) []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
