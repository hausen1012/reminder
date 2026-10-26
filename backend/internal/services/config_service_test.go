package services

import (
	"testing"
)

// 写入后读取应一致。
func TestConfigService_UpsertAndGet(t *testing.T) {
	db := openTestDB(t)
	svc := NewConfigService(db)

	if err := svc.UpsertMap(map[string]string{"a": "1", "b": "2"}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	got, err := svc.GetAll()
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got["a"] != "1" || got["b"] != "2" {
		t.Fatalf("读取不一致: %v", got)
	}

	// 覆盖
	if err := svc.UpsertMap(map[string]string{"a": "9"}); err != nil {
		t.Fatalf("覆盖失败: %v", err)
	}
	got, _ = svc.GetAll()
	if got["a"] != "9" {
		t.Fatalf("覆盖后 a = %q, 期望 9", got["a"])
	}
}

// 空值应删除对应 key。
func TestConfigService_EmptyValueDeletes(t *testing.T) {
	db := openTestDB(t)
	svc := NewConfigService(db)

	if err := svc.UpsertMap(map[string]string{"keep": "v", "drop": "v"}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	if err := svc.UpsertMap(map[string]string{"drop": ""}); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	got, _ := svc.GetAll()
	if _, ok := got["drop"]; ok {
		t.Fatalf("drop 应被删除，实际仍在: %v", got)
	}
	if got["keep"] != "v" {
		t.Fatalf("keep 不应受影响: %v", got)
	}
}
