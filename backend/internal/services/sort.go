package services

import "strings"

// buildOrderClause 把前端传入的 sort_by / sort_order 转成安全的 ORDER BY 片段。
//
// sort_by 会被直接拼进 SQL，因此必须限定在白名单内，防止注入。
// 未命中白名单或为空时回退到 fallback（形如 "created_at DESC"）。
func buildOrderClause(sortBy, sortOrder, fallback string, allowed []string) string {
	field := strings.ToLower(strings.TrimSpace(sortBy))
	if field == "" {
		return fallback
	}
	ok := false
	for _, a := range allowed {
		if field == a {
			ok = true
			break
		}
	}
	if !ok {
		return fallback
	}
	if strings.EqualFold(strings.TrimSpace(sortOrder), "asc") {
		return field + " ASC"
	}
	return field + " DESC"
}

// splitOrderClause 把 "field DIR" 拆成 (field, DIR, true)。
//
// 只接受 buildOrderClause 产出的形态（白名单字段 + ASC/DESC），
// 解析失败时返回 ok=false，调用方应原样使用原片段。
func splitOrderClause(clause string) (field, dir string, ok bool) {
	parts := strings.Fields(clause)
	if len(parts) != 2 {
		return "", "", false
	}
	if !strings.EqualFold(parts[1], "ASC") && !strings.EqualFold(parts[1], "DESC") {
		return "", "", false
	}
	return parts[0], strings.ToUpper(parts[1]), true
}
