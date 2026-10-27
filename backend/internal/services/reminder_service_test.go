package services

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// newReminderStack 构造不含确认机制的 ReminderService。
func newReminderStack(t *testing.T, db *gorm.DB) *ReminderService {
	t.Helper()
	loc := time.FixedZone("CST", 8*3600)
	return NewReminderService(db, noopEngine{}, testTZ(loc), newDispatchStack(t, db), nil)
}

// 最小合法入参：未来的一次性提醒。
func validOnceInput() ReminderInput {
	return ReminderInput{
		Title:        "测试提醒",
		Content:      "正文",
		Calendar:     "solar",
		ScheduleType: "once",
		ScheduleSpec: map[string]any{"at": "2099-01-01T09:00:00"},
	}
}

// 创建成功时：默认 source=web、content 回退为 title、enabled=true、next_fire_at 有值。
func TestReminderCreate_Defaults(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	in := validOnceInput()
	in.Content = "" // 未填正文时应回退为标题
	v, err := svc.Create(in)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	if v.Source != "web" {
		t.Fatalf("source = %q，期望 web", v.Source)
	}
	if v.Content != in.Title {
		t.Fatalf("content = %q，为空时应回退为 title", v.Content)
	}
	if v.ContentFormat != "text" {
		t.Fatalf("content_format = %q，期望默认 text", v.ContentFormat)
	}
	if !v.Enabled {
		t.Fatal("默认应启用")
	}
	if v.NextFireAt == nil {
		t.Fatal("next_fire_at 应有值")
	}
}

// 显式传 enabled=false 时必须生效（回归：gorm default:true 曾把它覆盖成 true）。
func TestReminderCreate_DisabledStaysDisabled(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	in := validOnceInput()
	disabled := false
	in.Enabled = &disabled

	v, err := svc.Create(in)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if v.Enabled {
		t.Fatal("创建时 enabled=false 被覆盖成了 true")
	}

	var got models.Reminder
	if err := db.First(&got, v.ID).Error; err != nil {
		t.Fatalf("重读失败: %v", err)
	}
	if got.Enabled {
		t.Fatal("数据库中的 enabled 应为 false")
	}
}

// 各类非法入参必须被拦截在写库之前。
func TestReminderCreate_Validation(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	cases := []struct {
		name   string
		mutate func(*ReminderInput)
	}{
		{"标题为空", func(in *ReminderInput) { in.Title = "   " }},
		{"标题超长", func(in *ReminderInput) { in.Title = string(make([]rune, 201)) }},
		{"calendar 非法", func(in *ReminderInput) { in.Calendar = "mayan" }},
		{"schedule_type 非法", func(in *ReminderInput) { in.ScheduleType = "weekly" }},
		{"lunar 不支持 cron", func(in *ReminderInput) {
			in.Calendar = "lunar"
			in.ScheduleType = "cron"
			in.ScheduleSpec = map[string]any{"expr": "0 9 * * *"}
		}},
		{"content_format 非法", func(in *ReminderInput) { in.ContentFormat = "pdf" }},
		{"schedule_spec 为空", func(in *ReminderInput) { in.ScheduleSpec = nil }},
		{"缺少 at 字段", func(in *ReminderInput) { in.ScheduleSpec = map[string]any{} }},
		{"重发间隔过小", func(in *ReminderInput) {
			in.RequireConfirm = true
			in.ConfirmRetryIntervalSec = 10
			in.ConfirmMaxRetries = 3
		}},
		{"最大重试次数为 0", func(in *ReminderInput) {
			in.RequireConfirm = true
			in.ConfirmRetryIntervalSec = 60
			in.ConfirmMaxRetries = 0
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validOnceInput()
			tc.mutate(&in)
			if _, err := svc.Create(in); err == nil {
				t.Fatal("期望校验失败")
			}
		})
	}

	// 全部失败后不应残留任何记录
	var count int64
	db.Model(&models.Reminder{}).Count(&count)
	if count != 0 {
		t.Fatalf("校验失败不应写库，实际残留 %d 条", count)
	}
}

// 创建时绑定通道，Get 应能读回 channel_ids。
func TestReminderCreate_BindsChannels(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)
	ch := createChannel(t, db, "log", true)

	in := validOnceInput()
	in.ChannelIDs = []uint{ch.ID, ch.ID} // 重复 ID 应去重
	v, err := svc.Create(in)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	got, err := svc.Get(v.ID)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(got.ChannelIDs) != 1 {
		t.Fatalf("channel_ids 去重后应为 1 个，实际 %v", got.ChannelIDs)
	}
}

// 更新应替换字段并重新计算 next_fire_at、同时刷新通道绑定。
func TestReminderUpdate_ReplacesFieldsAndChannels(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)
	ch1 := createChannel(t, db, "log", true)
	ch2 := createChannel(t, db, "log", true)

	in := validOnceInput()
	in.ChannelIDs = []uint{ch1.ID}
	created, err := svc.Create(in)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	upd := ReminderInput{
		Title:        "改后的标题",
		Content:      "改后的正文",
		Calendar:     "solar",
		ScheduleType: "once",
		ScheduleSpec: map[string]any{"at": "2099-06-01T10:30:00"},
		ChannelIDs:   []uint{ch2.ID},
	}
	v, err := svc.Update(created.ID, upd)
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	if v.Title != "改后的标题" || v.Content != "改后的正文" {
		t.Fatalf("字段未更新: title=%q content=%q", v.Title, v.Content)
	}
	if len(v.ChannelIDs) != 1 || v.ChannelIDs[0] != ch2.ID {
		t.Fatalf("通道绑定未替换: %v", v.ChannelIDs)
	}
}

// 更新不存在的提醒应返回 not_found。
func TestReminderUpdate_NotFound(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	if _, err := svc.Update(9999, validOnceInput()); err == nil {
		t.Fatal("更新不存在的提醒应报错")
	}
}

// Toggle 关闭时应清空 next_fire_at；再打开时应重新计算。
func TestReminderToggle(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	created, err := svc.Create(validOnceInput())
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	off, err := svc.Toggle(created.ID)
	if err != nil {
		t.Fatalf("关闭失败: %v", err)
	}
	if off.Enabled {
		t.Fatal("应已关闭")
	}
	if off.NextFireAt != nil {
		t.Fatalf("关闭后 next_fire_at 应为 nil，实际 %v", off.NextFireAt)
	}

	on, err := svc.Toggle(created.ID)
	if err != nil {
		t.Fatalf("重新开启失败: %v", err)
	}
	if !on.Enabled {
		t.Fatal("应已开启")
	}
	if on.NextFireAt == nil {
		t.Fatal("重新开启后 next_fire_at 应有值")
	}
}

// Delete 应同时清理关联表并删除提醒本体。
func TestReminderDelete_CleansJoinTable(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)
	ch := createChannel(t, db, "log", true)

	in := validOnceInput()
	in.ChannelIDs = []uint{ch.ID}
	created, err := svc.Create(in)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	if err := svc.Delete(created.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	var joinCount int64
	db.Model(&models.ReminderChannel{}).Where("reminder_id = ?", created.ID).Count(&joinCount)
	if joinCount != 0 {
		t.Fatalf("关联表未清理，残留 %d 条", joinCount)
	}

	if _, err := svc.Get(created.ID); err == nil {
		t.Fatal("删除后应查不到")
	}
}

// BatchDelete 应批量清理提醒与关联表。
func TestReminderBatchDelete(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)
	ch := createChannel(t, db, "log", true)

	var ids []uint
	for i := 0; i < 3; i++ {
		in := validOnceInput()
		in.Title = string(rune('A'+i)) + "提醒"
		in.ChannelIDs = []uint{ch.ID}
		v, err := svc.Create(in)
		if err != nil {
			t.Fatalf("创建失败: %v", err)
		}
		ids = append(ids, v.ID)
	}

	if err := svc.BatchDelete(ids[:2]); err != nil {
		t.Fatalf("批量删除失败: %v", err)
	}

	_, total, err := svc.List(ListFilter{})
	if err != nil {
		t.Fatalf("列表查询失败: %v", err)
	}
	if total != 1 {
		t.Fatalf("剩余 %d 条，期望 1 条", total)
	}

	var joinCount int64
	db.Model(&models.ReminderChannel{}).Where("reminder_id IN ?", ids[:2]).Count(&joinCount)
	if joinCount != 0 {
		t.Fatalf("关联表未清理，残留 %d 条", joinCount)
	}
}

// 列表：source / enabled / search 过滤与分页应生效。
func TestReminderList_FiltersAndPaging(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	// 3 条 web + 1 条 api，其中一条 api 被禁用
	for i := 0; i < 3; i++ {
		in := validOnceInput()
		in.Title = "web-" + string(rune('0'+i))
		if _, err := svc.Create(in); err != nil {
			t.Fatalf("创建失败: %v", err)
		}
	}
	apiIn := validOnceInput()
	apiIn.Title = "api-提醒"
	apiIn.Source = "api"
	disabled := false
	apiIn.Enabled = &disabled
	if _, err := svc.Create(apiIn); err != nil {
		t.Fatalf("创建 api 提醒失败: %v", err)
	}

	items, total, err := svc.List(ListFilter{Source: "web"})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("source=web 应返回 3 条，实际 total=%d len=%d", total, len(items))
	}

	enabled := true
	items, total, err = svc.List(ListFilter{Enabled: &enabled})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 3 {
		t.Fatalf("enabled=true 应返回 3 条，实际 %d", total)
	}

	_, total, err = svc.List(ListFilter{Search: "api-"})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 1 {
		t.Fatalf("搜索 api- 应返回 1 条，实际 %d", total)
	}

	items, total, err = svc.List(ListFilter{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if total != 4 {
		t.Fatalf("总数应为 4，实际 %d", total)
	}
	if len(items) != 2 {
		t.Fatalf("limit=2 应返回 2 条，实际 %d", len(items))
	}
}

// 非法排序字段应回退到默认排序而不是报错（配合白名单防注入）。
func TestReminderList_RejectsInjectedSort(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)
	if _, err := svc.Create(validOnceInput()); err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	_, _, err := svc.List(ListFilter{SortBy: "id; DROP TABLE reminders", SortOrder: "asc"})
	if err != nil {
		t.Fatalf("非法排序字段应回退默认排序而非报错: %v", err)
	}

	// 表仍然存在
	var count int64
	db.Model(&models.Reminder{}).Count(&count)
	if count != 1 {
		t.Fatalf("数据被破坏，剩余 %d 条", count)
	}
}
