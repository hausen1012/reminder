package version

import (
	"strconv"
	"strings"
)

// IsNewer 判断 latest 是否比 current 新。
//
// 只比较点分数字部分（如 v1.4.1 → 1.4.1），忽略预发布后缀；
// 任一版本无法解析（如本地开发版本的 dev）时返回 false。
func IsNewer(latest, current string) bool {
	l, ok := parse(latest)
	if !ok {
		return false
	}
	c, ok := parse(current)
	if !ok {
		return false
	}
	for i := 0; i < len(l) || i < len(c); i++ {
		var lv, cv int
		if i < len(l) {
			lv = l[i]
		}
		if i < len(c) {
			cv = c[i]
		}
		if lv != cv {
			return lv > cv
		}
	}
	return false
}

// Valid 判断版本号能否解析；用于过滤标签列表里无法比较的名称。
func Valid(v string) bool {
	_, ok := parse(v)
	return ok
}

// parse 把 v1.2.3-rc1 拆成 [1,2,3]；格式不合法时返回 false。
func parse(v string) ([]int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		nums = append(nums, n)
	}
	return nums, true
}
