package services

import (
	"time"

	"github.com/reminder/backend/internal/timezone"
)

// testTZ 把固定时区包装成 timezone.Provider，供测试注入。
func testTZ(loc *time.Location) *timezone.Provider {
	return timezone.NewFixed(loc)
}

// cstZone 是测试默认时区（东八区），与生产默认值一致。
func cstZone() *time.Location {
	return time.FixedZone("CST", 8*3600)
}
