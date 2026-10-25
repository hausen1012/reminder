package services

import "testing"

// 排序字段白名单：合法值生效，非法值一律回退到默认排序。
func TestBuildOrderClause(t *testing.T) {
	allowed := []string{"id", "created_at", "next_fire_at"}

	cases := []struct {
		name      string
		sortBy    string
		sortOrder string
		want      string
	}{
		{"空值回退默认", "", "", "created_at DESC"},
		{"合法字段升序", "id", "asc", "id ASC"},
		{"合法字段降序", "next_fire_at", "desc", "next_fire_at DESC"},
		{"大小写归一", "ID", "ASC", "id ASC"},
		{"首尾空格容忍", "  created_at  ", " asc ", "created_at ASC"},
		{"非白名单字段回退", "id; DROP TABLE reminders", "asc", "created_at DESC"},
		{"子查询注入回退", "(SELECT 1)", "asc", "created_at DESC"},
		{"未知字段回退", "password", "asc", "created_at DESC"},
		{"非法排序方向按降序", "id", "sideways", "id DESC"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildOrderClause(tc.sortBy, tc.sortOrder, "created_at DESC", allowed)
			if got != tc.want {
				t.Fatalf("buildOrderClause(%q, %q) = %q, 期望 %q", tc.sortBy, tc.sortOrder, got, tc.want)
			}
		})
	}
}
