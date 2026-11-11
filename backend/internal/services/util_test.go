package services

import (
	"testing"

	"github.com/reminder/backend/internal/models"
)

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

// 更新时正文留空应回退为标题，而不是把正文清空。
func TestReminderUpdate_EmptyContentFallsBackToTitle(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	in := validOnceInput()
	in.Content = "原始正文"
	created, err := svc.Create(in)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	upd := validOnceInput()
	upd.Title = "新标题"
	upd.Content = "" // 只改标题，不填正文

	v, err := svc.Update(created.ID, upd)
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if v.Content != "新标题" {
		t.Fatalf("正文应回退为新标题，实际 %q", v.Content)
	}
}

// 创建时正文为纯空白也应回退为标题（而非保留空白）。
func TestReminderCreate_BlankContentFallsBackToTitle(t *testing.T) {
	db := openTestDB(t)
	svc := newReminderStack(t, db)

	in := validOnceInput()
	in.Title = "标题"
	in.Content = "   "

	v, err := svc.Create(in)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if v.Content != "标题" {
		t.Fatalf("空白正文应回退为标题，实际 %q", v.Content)
	}
}

// 分页参数归一化：缺省/超限/负数都有确定结果。
func TestNormalizePage(t *testing.T) {
	cases := []struct {
		name              string
		limit, offset     int
		wantLimit, wantOf int
	}{
		{"缺省 limit", 0, 0, 50, 0},
		{"超限 limit", 500, 0, 200, 0},
		{"负数 offset", 10, -5, 10, 0},
		{"正常值原样保留", 20, 40, 20, 40},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limit, offset := normalizePage(tc.limit, tc.offset)
			if limit != tc.wantLimit || offset != tc.wantOf {
				t.Fatalf("normalizePage(%d, %d) = (%d, %d)，期望 (%d, %d)",
					tc.limit, tc.offset, limit, offset, tc.wantLimit, tc.wantOf)
			}
		})
	}
}

// 关联表批量查询与整表替换：批量结果按主体分组，替换时去重并丢弃 0。
func TestJoinHelpers(t *testing.T) {
	db := openTestDB(t)
	ch1 := createChannel(t, db, "log")
	ch2 := createChannel(t, db, "log")

	for _, rid := range []uint{1, 2} {
		if err := replaceJoinRows(db, &models.ReminderChannel{}, "reminder_id", rid,
			[]uint{ch1.ID, ch2.ID, ch1.ID, 0},
			func(ownerID, targetID uint) models.ReminderChannel {
				return models.ReminderChannel{ReminderID: ownerID, ChannelID: targetID}
			}); err != nil {
			t.Fatalf("替换关联失败: %v", err)
		}
	}

	got, err := joinIDs(db, &models.ReminderChannel{}, "reminder_id", 1, "channel_id")
	if err != nil {
		t.Fatalf("查询关联失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("去重后应剩 2 条关联，实际 %v", got)
	}

	batch, err := joinIDsBatch(db, &models.ReminderChannel{}, "reminder_id", "channel_id", []uint{1, 2})
	if err != nil {
		t.Fatalf("批量查询失败: %v", err)
	}
	if len(batch[1]) != 2 || len(batch[2]) != 2 {
		t.Fatalf("批量结果 = %v", batch)
	}

	if err := replaceJoinRows(db, &models.ReminderChannel{}, "reminder_id", 1, nil,
		func(ownerID, targetID uint) models.ReminderChannel {
			return models.ReminderChannel{ReminderID: ownerID, ChannelID: targetID}
		}); err != nil {
		t.Fatalf("清空关联失败: %v", err)
	}
	got, _ = joinIDs(db, &models.ReminderChannel{}, "reminder_id", 1, "channel_id")
	if len(got) != 0 {
		t.Fatalf("清空后应为空，实际 %v", got)
	}

	if _, err := firstByID[models.Channel](db, 9999, "通道不存在"); err == nil {
		t.Fatal("不存在的记录应返回 404")
	}
}
