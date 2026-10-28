package models

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// openTestDB 打开一个临时 SQLite 库，供本文件的时间类型用例共用。
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "time.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取 sql.DB 失败: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// 时间必须以整数毫秒落库——这是摆脱"文本字典序 + 偏移错位"的结构性前提。
func TestTime_StoredAsMillisecondInteger(t *testing.T) {
	db := openTestDB(t)
	if err := db.AutoMigrate(&Reminder{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	var cols []struct {
		Name string `gorm:"column:name"`
		Type string `gorm:"column:type"`
	}
	if err := db.Raw("PRAGMA table_info(reminders)").Scan(&cols).Error; err != nil {
		t.Fatalf("读取表结构失败: %v", err)
	}
	want := map[string]string{
		"next_fire_at":  "INTEGER",
		"last_fired_at": "INTEGER",
		"created_at":    "INTEGER",
		"updated_at":    "INTEGER",
	}
	for _, c := range cols {
		if exp, ok := want[c.Name]; ok && c.Type != exp {
			t.Fatalf("列 %s 类型 = %q，期望 %q", c.Name, c.Type, exp)
		}
	}
	if len(cols) == 0 {
		t.Fatal("未读到表结构")
	}
}

// 落库值应等于 UnixMillis，而不是任何文本形态。
func TestTime_WriteAndReadRoundTrip(t *testing.T) {
	db := openTestDB(t)
	if err := db.AutoMigrate(&Reminder{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	// 故意用带偏移的时间：TimeOf 应归一化为 UTC
	loc := time.FixedZone("CST", 8*3600)
	want := time.Date(2026, 6, 26, 12, 0, 0, 123_000_000, loc)
	r := &Reminder{
		Title:        "往返",
		Calendar:     "solar",
		ScheduleType: "once",
		Enabled:      true,
		NextFireAt:   TimePtrOf(want),
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	var raw struct {
		Next    *int64 `gorm:"column:next_fire_at"`
		Created *int64 `gorm:"column:created_at"`
	}
	if err := db.Raw("SELECT CAST(next_fire_at AS INTEGER) AS next_fire_at, CAST(created_at AS INTEGER) AS created_at FROM reminders WHERE id = ?", r.ID).
		Scan(&raw).Error; err != nil {
		t.Fatalf("读取原始值失败: %v", err)
	}
	if raw.Next == nil {
		t.Fatal("next_fire_at 应为整数而非 NULL")
	}
	if wantMs := want.UTC().UnixMilli(); *raw.Next != wantMs {
		t.Fatalf("落库值 = %d，期望 %d", *raw.Next, wantMs)
	}
	if raw.Created == nil || *raw.Created == 0 {
		t.Fatalf("created_at 未自动填充，得到 %v", raw.Created)
	}

	var got Reminder
	if err := db.First(&got, r.ID).Error; err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if got.NextFireAt == nil {
		t.Fatal("读回的 next_fire_at 为空")
	}
	if !got.NextFireAt.Equal(want) {
		t.Fatalf("读回时间 = %v，期望 %v", got.NextFireAt.Time, want)
	}
	// 精度是毫秒：纳秒会被截断，这里断言差异不超过 1ms
	if diff := got.NextFireAt.Sub(want); diff < -time.Millisecond || diff > time.Millisecond {
		t.Fatalf("读回时间与写入相差 %v，超出毫秒精度", diff)
	}
}

// 可空字段：nil 写 NULL、读回 nil；非 nil 正常往返。
func TestTime_NullablePointer(t *testing.T) {
	db := openTestDB(t)
	if err := db.AutoMigrate(&Reminder{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	empty := &Reminder{Title: "无时间", Calendar: "solar", ScheduleType: "once"}
	if err := db.Create(empty).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	var nullCount int64
	if err := db.Raw("SELECT COUNT(*) FROM reminders WHERE next_fire_at IS NULL").Scan(&nullCount).Error; err != nil {
		t.Fatalf("统计 NULL 失败: %v", err)
	}
	if nullCount != 1 {
		t.Fatalf("空指针应写 NULL，实际 NULL 行数 = %d", nullCount)
	}
	var gotEmpty Reminder
	if err := db.First(&gotEmpty, empty.ID).Error; err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if gotEmpty.NextFireAt != nil {
		t.Fatalf("NULL 应读回 nil，实际 %v", gotEmpty.NextFireAt)
	}

	// map 更新成 nil 也要写成 NULL，而不是零值时间
	at := time.Now().UTC()
	one := &Reminder{Title: "有时间", Calendar: "solar", ScheduleType: "once", NextFireAt: TimePtrOf(at)}
	if err := db.Create(one).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := db.Model(&Reminder{}).Where("id = ?", one.ID).Update("next_fire_at", nil).Error; err != nil {
		t.Fatalf("置空失败: %v", err)
	}
	var gotOne Reminder
	if err := db.First(&gotOne, one.ID).Error; err != nil {
		t.Fatalf("读回失败: %v", err)
	}
	if gotOne.NextFireAt != nil {
		t.Fatalf("置空后应读回 nil，实际 %v", gotOne.NextFireAt)
	}
}

// 排序与范围比较是整数语义：时刻大小即数值大小，与写入时的偏移无关。
func TestTime_OrderingIgnoresOriginalOffset(t *testing.T) {
	db := openTestDB(t)
	if err := db.AutoMigrate(&Reminder{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	base := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	cst := time.FixedZone("CST", 8*3600)
	// 同一时刻的两种写法，加上前后各一个时刻；打乱插入顺序
	inputs := []struct {
		title string
		at    time.Time
	}{
		{"晚", base.Add(time.Hour).In(cst)},
		{"早", base.Add(-time.Hour)},
		{"同刻-CST", base.In(cst)},
		{"同刻-UTC", base},
	}
	for _, in := range inputs {
		r := &Reminder{Title: in.title, Calendar: "solar", ScheduleType: "once", NextFireAt: TimePtrOf(in.at)}
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("创建失败: %v", err)
		}
	}

	var rows []Reminder
	if err := db.Order("next_fire_at ASC").Find(&rows).Error; err != nil {
		t.Fatalf("排序查询失败: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("应读到 4 行，实际 %d", len(rows))
	}
	// 最早的是一小时前，最后是一小时后；中间两个是同一时刻
	if rows[0].Title != "早" {
		t.Fatalf("首行应为「早」，实际 %q", rows[0].Title)
	}
	if rows[3].Title != "晚" {
		t.Fatalf("末行应为「晚」，实际 %q", rows[3].Title)
	}
	if !rows[1].NextFireAt.Equal(rows[2].NextFireAt.Std()) {
		t.Fatalf("中间两行应为同一时刻，实际 %v / %v", rows[1].NextFireAt.Time, rows[2].NextFireAt.Time)
	}

	// 范围比较：同一时刻的两种写法都应命中（文本方案下这里正是错序的来源）
	var inRange []Reminder
	if err := db.Where("next_fire_at = ?", TimeOf(base)).Find(&inRange).Error; err != nil {
		t.Fatalf("等值查询失败: %v", err)
	}
	if len(inRange) != 2 {
		t.Fatalf("同一时刻应有 2 行，实际 %d", len(inRange))
	}
}

// 等值是精确的毫秒匹配，不会因为浮点分辨率把相邻毫秒判成相等。
func TestTime_EqualityIsExact(t *testing.T) {
	db := openTestDB(t)
	if err := db.AutoMigrate(&Reminder{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	base := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		at := base.Add(time.Duration(i) * time.Millisecond)
		r := &Reminder{Title: "等值", Calendar: "solar", ScheduleType: "once", NextFireAt: TimePtrOf(at)}
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("创建失败: %v", err)
		}
	}

	// 乐观锁依赖的就是这种精确等值：命中必须只有 1 行
	var n int64
	if err := db.Model(&Reminder{}).Where("next_fire_at = ?", TimeOf(base.Add(time.Millisecond))).Count(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("毫秒级等值应命中 1 行，实际 %d（说明比较不够精确）", n)
	}
}

// MarshalJSON 输出 RFC3339 字符串，UnmarshalJSON 能读回同一时刻。
func TestTime_JSONRoundTrip(t *testing.T) {
	at := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)

	got, err := json.Marshal(TimeOf(at))
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if string(got) != `"2026-06-26T12:00:00Z"` {
		t.Fatalf("JSON = %s，期望 RFC3339 字符串", got)
	}

	var back Time
	if err := json.Unmarshal(got, &back); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if !back.Equal(at) {
		t.Fatalf("反序列化得到 %v，期望 %v", back.Time, at)
	}

	// 可空字段：nil 指针序列化为 null，反序列化回来仍是 nil
	var p *Time
	gotNil, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("序列化 nil 失败: %v", err)
	}
	if string(gotNil) != "null" {
		t.Fatalf("nil 指针应序列化为 null，实际 %s", gotNil)
	}
	var backNil *Time
	if err := json.Unmarshal([]byte("null"), &backNil); err != nil {
		t.Fatalf("反序列化 null 失败: %v", err)
	}
	if backNil != nil {
		t.Fatalf("null 应读回 nil，实际 %v", backNil)
	}

	// 带毫秒与小数的写法也应解析成功，并截断到毫秒
	var withMs Time
	if err := json.Unmarshal([]byte(`"2026-06-26T12:00:00.123456789+08:00"`), &withMs); err != nil {
		t.Fatalf("反序列化带小数的时间失败: %v", err)
	}
	if want := time.Date(2026, 6, 26, 4, 0, 0, 123000000, time.UTC); !withMs.Equal(want) {
		t.Fatalf("反序列化得到 %v，期望 %v", withMs.Time, want)
	}

	// 空串等同"无此时间"
	var backEmpty Time
	if err := json.Unmarshal([]byte(`""`), &backEmpty); err != nil {
		t.Fatalf("反序列化空串失败: %v", err)
	}
	if !backEmpty.IsZero() {
		t.Fatalf("空串应读成零值，实际 %v", backEmpty.Time)
	}
}

// Scan 支持整数毫秒、time.Time 与 NULL。
func TestTime_Scan(t *testing.T) {
	cases := []struct {
		name string
		src  any
		want time.Time
	}{
		{"整数毫秒", int64(1782475200123), time.UnixMilli(1782475200123).UTC()},
		{"time.Time", time.Date(2026, 6, 26, 12, 0, 0, 123000000, time.UTC), time.Date(2026, 6, 26, 12, 0, 0, 123000000, time.UTC)},
		{"NULL", nil, time.Time{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got Time
			if err := got.Scan(tc.src); err != nil {
				t.Fatalf("Scan 返回错误: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("Scan(%v) = %v，期望 %v", tc.src, got.Time, tc.want)
			}
		})
	}
}
