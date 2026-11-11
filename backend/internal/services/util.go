// util 提供分批查询与排序白名单等辅助工具函数。
package services

import "strings"

// chunkIDs 将 ID 切片按 size 分批，避免 SQLite 的 too many SQL variables 错误。
func chunkIDs(ids []uint, size int) [][]uint {
	if len(ids) == 0 {
		return nil
	}
	var chunks [][]uint
	for i := 0; i < len(ids); i += size {
		end := i + size
		if end > len(ids) {
			end = len(ids)
		}
		chunks = append(chunks, ids[i:end])
	}
	return chunks
}

// buildOrderClause 把前端传入的 sort_by / sort_order 转成白名单校验后的 ORDER BY 片段。
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

// 列表接口分页参数的统一边界。
const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

// normalizePage 归一化分页参数：limit 缺省为 50、上限 200，offset 不得为负。
func normalizePage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultPageLimit
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
