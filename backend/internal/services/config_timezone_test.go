package services

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/timezone"
)

// 未在库里设置过时区时，GetAll 补上当前生效值，让设置页有可编辑的初始值。
func TestConfigService_GetAllIncludesTimezoneDefault(t *testing.T) {
	db := openTestDB(t)
	svc := NewConfigService(db, timezone.NewFixed(time.FixedZone("CST", 8*3600)))

	got, err := svc.GetAll()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got[KeyTimezone] == "" {
		t.Fatalf("应回显当前时区，实际 %v", got)
	}
}

// 写入合法时区后：落库、Provider 热更新、回调被触发。
func TestConfigService_UpdateTimezoneApplies(t *testing.T) {
	db := openTestDB(t)
	tz := timezone.NewFixed(time.FixedZone("CST", 8*3600))
	svc := NewConfigService(db, tz)

	called := 0
	svc.OnTimezoneChanged = func() { called++ }

	if err := svc.UpsertMap(map[string]string{KeyTimezone: "America/New_York"}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if tz.Name() != "America/New_York" {
		t.Fatalf("Provider 未热更新，实际 %q", tz.Name())
	}
	if called != 1 {
		t.Fatalf("回调应触发 1 次，实际 %d", called)
	}

	// 落库的是新值
	var row struct{ Value string }
	if err := db.Raw("SELECT value FROM configs WHERE key = ?", KeyTimezone).Scan(&row).Error; err != nil {
		t.Fatalf("读库失败: %v", err)
	}
	if row.Value != "America/New_York" {
		t.Fatalf("落库值 = %q", row.Value)
	}
}

// 非法时区应被拒绝，且不改变当前生效值、不写库。
func TestConfigService_InvalidTimezoneRejected(t *testing.T) {
	db := openTestDB(t)
	tz := timezone.NewFixed(time.FixedZone("CST", 8*3600))
	svc := NewConfigService(db, tz)

	err := svc.UpsertMap(map[string]string{KeyTimezone: "Mars/Olympus"})
	if err == nil {
		t.Fatal("非法时区应报错")
	}
	appErr, ok := err.(*middleware.AppError)
	if !ok || appErr.Code != middleware.CodeValidationFailed {
		t.Fatalf("应为校验错误，实际 %v", err)
	}
	if appErr.Field != KeyTimezone {
		t.Fatalf("field = %q，期望 %q", appErr.Field, KeyTimezone)
	}
	if tz.Name() == "Mars/Olympus" {
		t.Fatal("非法时区不应生效")
	}
	got, _ := svc.GetAll()
	if got[KeyTimezone] == "Mars/Olympus" {
		t.Fatal("非法时区不应落库")
	}
}

// 启动时 LoadTimezone 应把库里的时区应用到 Provider。
func TestConfigService_LoadTimezone(t *testing.T) {
	db := openTestDB(t)
	tz := timezone.NewFixed(time.FixedZone("CST", 8*3600))
	svc := NewConfigService(db, tz)

	// 库里没有配置：保留环境变量给的默认值
	svc.LoadTimezone()
	if tz.Name() != "CST" {
		t.Fatalf("无配置时应保留默认值，实际 %q", tz.Name())
	}

	// 直接写库（模拟老数据），再启动加载
	if err := db.Exec("INSERT INTO configs (key, value) VALUES (?, ?)", KeyTimezone, "Europe/Berlin").Error; err != nil {
		t.Fatalf("写库失败: %v", err)
	}
	svc.LoadTimezone()
	if tz.Name() != "Europe/Berlin" {
		t.Fatalf("应从库加载时区，实际 %q", tz.Name())
	}
}

// 库里的时区非法时保留默认值并告警，不影响启动。
func TestConfigService_LoadTimezoneInvalid(t *testing.T) {
	db := openTestDB(t)
	tz := timezone.NewFixed(time.FixedZone("CST", 8*3600))
	svc := NewConfigService(db, tz)

	if err := db.Exec("INSERT INTO configs (key, value) VALUES (?, ?)", KeyTimezone, "Nowhere/Land").Error; err != nil {
		t.Fatalf("写库失败: %v", err)
	}
	svc.LoadTimezone()
	if tz.Name() != "CST" {
		t.Fatalf("非法配置应保留默认值，实际 %q", tz.Name())
	}
}

// 清空时区配置应回到默认时区，并仍然刷新调度器。
func TestConfigService_ClearTimezoneFallsBack(t *testing.T) {
	db := openTestDB(t)
	tz := timezone.NewFixed(time.FixedZone("CST", 8*3600))
	svc := NewConfigService(db, tz)

	called := 0
	svc.OnTimezoneChanged = func() { called++ }

	if err := svc.UpsertMap(map[string]string{KeyTimezone: "America/New_York"}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if err := svc.UpsertMap(map[string]string{KeyTimezone: ""}); err != nil {
		t.Fatalf("清空失败: %v", err)
	}
	if tz.Name() != timezone.Fallback {
		t.Fatalf("清空后应回落到 %q，实际 %q", timezone.Fallback, tz.Name())
	}
	if called != 2 {
		t.Fatalf("两次变更都应触发回调，实际 %d", called)
	}
}
