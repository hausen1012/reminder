package services

import (
	"testing"

	"github.com/reminder/backend/internal/crypto/secretbox"
)

// 部分更新应保留未提交的字段，而不是把整份 config 覆盖成只剩提交的字段。
//
// 这是一个回归用例：Update 只把 patch 里的字段加密后整体写回，
// 若调用方只提交 config 的一个子集（前端「只改某一项」的场景），
// 其余字段会被静默丢弃。
func TestChannelUpdate_PartialKeepsOtherFields(t *testing.T) {
	db := openTestDB(t)
	box, _ := secretbox.New("")
	svc := NewChannelService(db, box)

	created, err := svc.Create(ChannelInput{
		Name: "partial", Type: "smtp",
		Config: map[string]any{
			"host": "smtp.test", "port": 587, "username": "u",
			"password_enc": "orig", "from_addr": "f@test", "to": []string{"t@test"},
		},
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	v, err := svc.Update(created.ID, ChannelInput{
		Name:   "partial",
		Config: map[string]any{"from_name": "新名字"},
	})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	if v.Config["host"] != "smtp.test" {
		t.Fatalf("host 应保留，实际 %v（整份 config: %v）", v.Config["host"], v.Config)
	}
	if v.Config["port"] == nil {
		t.Fatalf("port 应保留，实际 %v", v.Config)
	}
	if v.Config["from_name"] != "新名字" {
		t.Fatalf("from_name 应更新，实际 %v", v.Config["from_name"])
	}
}
