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

// seedReminderForUpcoming 插入一条带 next_fire_at 的启用提醒。
func seedReminderForUpcoming(t *testing.T, db *gorm.DB, title string, next time.Time, enabled bool) *models.Reminder {
	t.Helper()
	r := &models.Reminder{
		Title:        title,
		Calendar:     "solar",
		ScheduleType: "once",
		Enabled:      true,
		Source:       "web",
		NextFireAt:   models.TimePtrOf(next),
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	if !enabled {
		if err := db.Model(&models.Reminder{}).Where("id = ?", r.ID).
			Update("enabled", false).Error; err != nil {
			t.Fatalf("禁用提醒失败: %v", err)
		}
	}
	return r
}

// Upcoming 只返回窗口内、已启用、有 next_fire_at 的提醒，按时间升序。
func TestReminderUpcoming(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	now := time.Now()
	seedReminderForUpcoming(t, db, "1小时后", now.Add(time.Hour), true)
	seedReminderForUpcoming(t, db, "10小时后", now.Add(10*time.Hour), true)
	seedReminderForUpcoming(t, db, "2天后", now.Add(48*time.Hour), true) // 超出 24h 窗口
	seedReminderForUpcoming(t, db, "已禁用", now.Add(2*time.Hour), false)

	items, err := svc.Upcoming(24*time.Hour, 10)
	if err != nil {
		t.Fatalf("Upcoming 失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("应返回 2 条，实际 %d", len(items))
	}
	if items[0].Title != "1小时后" {
		t.Fatalf("应按时间升序，首条 = %q", items[0].Title)
	}
	if items[1].Title != "10小时后" {
		t.Fatalf("第二条 = %q", items[1].Title)
	}
}

// Upcoming 的 limit：<=0 时默认 10，超过 50 时截断为 50。
func TestReminderUpcoming_LimitClamped(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	now := time.Now()
	for i := 0; i < 3; i++ {
		seedReminderForUpcoming(t, db, "r", now.Add(time.Duration(i+1)*time.Minute), true)
	}

	// limit=0 → 默认 10，应返回全部 3 条
	items, err := svc.Upcoming(time.Hour, 0)
	if err != nil {
		t.Fatalf("Upcoming 失败: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("应返回 3 条，实际 %d", len(items))
	}
}

// UpcomingBetween 返回指定区间内的提醒，limit=0 表示不限。
func TestReminderUpcomingBetween(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	base := time.Date(2030, 6, 15, 12, 0, 0, 0, time.UTC)
	seedReminderForUpcoming(t, db, "区间内", base, true)
	seedReminderForUpcoming(t, db, "区间外", base.Add(72*time.Hour), true)

	from := base.Add(-time.Hour)
	to := base.Add(time.Hour)
	items, err := svc.UpcomingBetween(from, to, 0)
	if err != nil {
		t.Fatalf("UpcomingBetween 失败: %v", err)
	}
	if len(items) != 1 || items[0].Title != "区间内" {
		t.Fatalf("应只返回区间内的 1 条，实际 %v", items)
	}
}

// Preview 计算未来 n 次触发；n 会被夹在 [1, 20]。
func TestReminderPreview(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	spec := map[string]any{"expr": "0 9 * * *"}
	times, err := svc.Preview("solar", "cron", spec, 3)
	if err != nil {
		t.Fatalf("Preview 失败: %v", err)
	}
	if len(times) != 3 {
		t.Fatalf("应返回 3 个时间，实际 %d", len(times))
	}

	// n=0 走默认 5
	times, err = svc.Preview("solar", "cron", spec, 0)
	if err != nil {
		t.Fatalf("Preview 失败: %v", err)
	}
	if len(times) != 5 {
		t.Fatalf("n=0 应默认 5 个，实际 %d", len(times))
	}

	// n 超过 20 应截断
	times, err = svc.Preview("solar", "cron", spec, 100)
	if err != nil {
		t.Fatalf("Preview 失败: %v", err)
	}
	if len(times) != 20 {
		t.Fatalf("n=100 应截断为 20，实际 %d", len(times))
	}

	// 非法 spec 应报错
	if _, err := svc.Preview("solar", "cron", map[string]any{"expr": "not a cron"}, 3); err == nil {
		t.Fatal("非法 cron 应报错")
	}
}

// Preview 按站点时区计算：改动 Provider 后结果随之变化。
func TestReminderPreview_UsesSiteTimezone(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	// 默认测试栈是东八区：09:00 应落在 CST 的 9 点
	times, err := svc.Preview("solar", "cron", map[string]any{"expr": "0 9 * * *"}, 2)
	if err != nil {
		t.Fatalf("Preview 失败: %v", err)
	}
	for _, tm := range times {
		if h := tm.In(cstZone()).Hour(); h != 9 {
			t.Fatalf("应按站点时区 9 点触发，实际 %d 点 (%s)", h, tm)
		}
	}

	// 切换站点时区后，同一条表达式应按新时区解释
	if err := svc.TZ.Set("America/New_York"); err != nil {
		t.Fatalf("切换时区失败: %v", err)
	}
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("加载时区失败: %v", err)
	}
	times, err = svc.Preview("solar", "cron", map[string]any{"expr": "0 9 * * *"}, 2)
	if err != nil {
		t.Fatalf("Preview 失败: %v", err)
	}
	for _, tm := range times {
		if h := tm.In(loc).Hour(); h != 9 {
			t.Fatalf("切换后应按纽约时区 9 点触发，实际 %d 点 (%s)", h, tm)
		}
	}
}

// TestDryRun：无通道、空标题、通道不存在、正常发送。
func TestReminderTestDryRun(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	if err := svc.TestDryRun(t.Context(), "t", "c", "text", nil); err == nil {
		t.Fatal("无通道应报错")
	}
	if err := svc.TestDryRun(t.Context(), "  ", "c", "text", []uint{1}); err == nil {
		t.Fatal("空标题应报错")
	}
	if err := svc.TestDryRun(t.Context(), "t", "c", "text", []uint{9999}); err == nil {
		t.Fatal("通道不存在应报错")
	}

	ch := createChannel(t, db, "log", true)
	if err := svc.TestDryRun(t.Context(), "试发", "正文 {{now_date}}", "text", []uint{ch.ID}); err != nil {
		t.Fatalf("正常试发应成功: %v", err)
	}
}

// loadChannelsByIDs 只加载 enabled 的通道。
func TestLoadChannelsByIDs(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	enabled := createChannel(t, db, "log", true)
	disabled := createChannel(t, db, "log", false)

	got, err := svc.loadChannelsByIDs([]uint{enabled.ID, disabled.ID})
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	if len(got) != 1 || got[0].ID != enabled.ID {
		t.Fatalf("应只加载 enabled 的 1 个通道，实际 %d 个", len(got))
	}

	// 空输入返回 nil
	got, err = svc.loadChannelsByIDs(nil)
	if err != nil || got != nil {
		t.Fatalf("空输入应返回 nil, nil，实际 %v, %v", got, err)
	}
}

// 列表类接口的查询数不应随返回条数增长（防 N+1）。
//
// 3 条提醒绑定通道后，List 应固定为 3 次查询：count + 分页 + 通道批量。
// 逐条查 channel_ids 会变成 3+N 次。
func TestReminderList_QueryCountIndependentOfRowCount(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	ch := createChannel(t, db, "log", true)
	for i := 0; i < 5; i++ {
		in := validOnceInput()
		in.Title = "n-plus-one-" + itoaTest(i)
		in.ChannelIDs = []uint{ch.ID}
		if _, err := svc.Create(in); err != nil {
			t.Fatalf("创建失败: %v", err)
		}
	}

	var got []*ReminderView
	n := countQueries(t, db, func(scoped *gorm.DB) {
		scopedSvc := &ReminderService{DB: scoped, TZ: svc.TZ}
		items, total, err := scopedSvc.List(ListFilter{Limit: 50})
		if err != nil {
			t.Fatalf("List 失败: %v", err)
		}
		if total != 5 || len(items) != 5 {
			t.Fatalf("应返回 5 条，实际 total=%d len=%d", total, len(items))
		}
		got = items
	})
	if n > 3 {
		t.Fatalf("List 产生了 %d 次查询，期望 ≤3（count + 分页 + 通道批量）", n)
	}
	for _, v := range got {
		if len(v.ChannelIDs) != 1 || v.ChannelIDs[0] != ch.ID {
			t.Fatalf("批量加载的 channel_ids 不正确: %v", v.ChannelIDs)
		}
	}
}

// Upcoming 同理：条数增长不应带来逐条查询。
func TestReminderUpcoming_QueryCountIndependentOfRowCount(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	ch := createChannel(t, db, "log", true)
	for i := 0; i < 5; i++ {
		r := seedReminderForUpcoming(t, db, "up-"+itoaTest(i), time.Now().Add(time.Minute), true)
		if err := db.Create(&models.ReminderChannel{ReminderID: r.ID, ChannelID: ch.ID}).Error; err != nil {
			t.Fatalf("绑定通道失败: %v", err)
		}
	}

	n := countQueries(t, db, func(scoped *gorm.DB) {
		scopedSvc := &ReminderService{DB: scoped, TZ: svc.TZ}
		items, err := scopedSvc.Upcoming(time.Hour, 10)
		if err != nil {
			t.Fatalf("Upcoming 失败: %v", err)
		}
		if len(items) != 5 {
			t.Fatalf("应返回 5 条，实际 %d", len(items))
		}
	})
	if n > 2 {
		t.Fatalf("Upcoming 产生了 %d 次查询，期望 ≤2（分页 + 通道批量）", n)
	}
}
