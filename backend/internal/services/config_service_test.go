package services

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/timezone"
)

// 写入后读取应一致。
func TestConfigService_UpsertAndGet(t *testing.T) {
	db := openTestDB(t)
	svc := NewConfigService(db, testTZ(time.FixedZone("CST", 8*3600)))

	if err := svc.UpsertMap(map[string]string{"app_name": "提醒", "logo_svg": "<svg/>"}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	got, err := svc.GetAll()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got["app_name"] != "提醒" || got["logo_svg"] != "<svg/>" {
		t.Fatalf("读取不一致: %v", got)
	}

	// 覆盖
	if err := svc.UpsertMap(map[string]string{"app_name": "改名"}); err != nil {
		t.Fatalf("覆盖失败: %v", err)
	}
	got, _ = svc.GetAll()
	if got["app_name"] != "改名" {
		t.Fatalf("覆盖后 app_name = %q, 期望 改名", got["app_name"])
	}
}

// 空值应删除对应 key。
func TestConfigService_EmptyValueDeletes(t *testing.T) {
	db := openTestDB(t)
	svc := NewConfigService(db, testTZ(time.FixedZone("CST", 8*3600)))

	if err := svc.UpsertMap(map[string]string{"app_name": "提醒", "logo_svg": "v"}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if err := svc.UpsertMap(map[string]string{"logo_svg": ""}); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	got, _ := svc.GetAll()
	if _, ok := got["logo_svg"]; ok {
		t.Fatalf("logo_svg 应被删除，实际仍在: %v", got)
	}
	if got["app_name"] != "提醒" {
		t.Fatalf("app_name 不应受影响: %v", got)
	}
}

// 重复保存同一个时区不应反复重建调度器：
// 设置页每次保存都会回传全部配置，未改时区时不该打扰调度器。
func TestConfigService_SameTimezoneSkipsCallback(t *testing.T) {
	db := openTestDB(t)
	// 必须用能被 time.LoadLocation 解析的时区名：这里要真的走一遍校验
	tz := timezone.New("Asia/Shanghai")
	svc := NewConfigService(db, tz)

	called := 0
	svc.OnTimezoneChanged = func() { called++ }

	// 与当前生效值相同（首次写入也算"没变化"）
	if err := svc.UpsertMap(map[string]string{KeyTimezone: "Asia/Shanghai"}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if called != 0 {
		t.Fatalf("时区未变化时不应触发重建，实际 %d 次", called)
	}

	// 只改其他配置也不该触发
	if err := svc.UpsertMap(map[string]string{KeyTimezone: "Asia/Shanghai", "app_name": "提醒"}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if called != 0 {
		t.Fatalf("未改时区不应触发重建，实际 %d 次", called)
	}

	// 真改了才触发
	if err := svc.UpsertMap(map[string]string{KeyTimezone: "America/New_York"}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if called != 1 {
		t.Fatalf("时区变化应触发 1 次重建，实际 %d", called)
	}
}

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

	// 直接写库，再启动加载
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

// 白名单之外的 key 必须被拒绝：否则可以借通用配置接口绕过
// /api/ai/settings 的加密与校验，把 AI 密钥等敏感内容直接写进 config 表。
func TestConfigService_UpsertRejectsUnknownKey(t *testing.T) {
	db := openTestDB(t)
	svc := NewConfigService(db, testTZ(time.FixedZone("CST", 8*3600)))

	err := svc.UpsertMap(map[string]string{"app_name": "提醒", "ai_api_key_enc": "明文密钥"})
	if err == nil {
		t.Fatal("白名单之外的 key 应报错")
	}
	appErr, ok := err.(*middleware.AppError)
	if !ok || appErr.Code != middleware.CodeValidationFailed {
		t.Fatalf("应为校验错误，实际 %v", err)
	}
	if appErr.Field != "ai_api_key_enc" {
		t.Fatalf("field = %q，期望 ai_api_key_enc", appErr.Field)
	}

	// 整批都不应落库：校验发生在写库之前
	got, _ := svc.GetAll()
	if _, ok := got["app_name"]; ok {
		t.Fatalf("校验失败时同批次的其他 key 也不应写入: %v", got)
	}
	var count int64
	db.Table("configs").Where("key = ?", "ai_api_key_enc").Count(&count)
	if count != 0 {
		t.Fatal("非法 key 不应写入数据库")
	}
}

// GetAll 只返回白名单内的 key，不把 AI 设置等内部配置带到浏览器。
func TestConfigService_GetAllFiltersUnknownKeys(t *testing.T) {
	db := openTestDB(t)
	svc := NewConfigService(db, testTZ(time.FixedZone("CST", 8*3600)))

	// 绕过服务直接写库，模拟历史数据或其它写入路径留下的内部配置
	rows := []struct{ k, v string }{
		{"app_name", "提醒"},
		{"ai_api_key_enc", "ENCRYPTED"},
		{"ai_provider", "openai"},
	}
	for _, r := range rows {
		if err := db.Exec("INSERT INTO configs (key, value) VALUES (?, ?)", r.k, r.v).Error; err != nil {
			t.Fatalf("写库失败: %v", err)
		}
	}

	got, err := svc.GetAll()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got["app_name"] != "提醒" {
		t.Fatalf("白名单内的 key 应返回: %v", got)
	}
	for _, k := range []string{"ai_api_key_enc", "ai_provider"} {
		if _, ok := got[k]; ok {
			t.Fatalf("%s 不应出现在 /api/config 的返回里: %v", k, got)
		}
	}
}
