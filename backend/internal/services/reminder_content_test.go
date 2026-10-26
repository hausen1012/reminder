package services

import (
	"testing"
)

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
