package models

import (
	"testing"

	"gorm.io/datatypes"
)

// 回归测试：Enabled 字段带 gorm default:true，若 Create 时传 false，
// GORM 会用默认值覆盖，导致「创建时禁用」静默变成「启用」。
//
// 原先放在 scheduler 包，但它测的是模型层的 GORM 默认值语义，与调度无关，
// 因此移到 models 包。本包无法复用 internal/testutil（它会反向导入 models）。
func TestReminderCreateWithDisabledIsNotOverridden(t *testing.T) {
	db := openTestDB(t)
	if err := db.AutoMigrate(&Reminder{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	r := &Reminder{
		Title:        "创建即禁用",
		Calendar:     "solar",
		ScheduleType: "cron",
		ScheduleSpec: datatypes.JSON(`{"expr":"0 9 * * *"}`),
		Enabled:      false,
		Source:       "web",
	}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	var got Reminder
	if err := db.First(&got, r.ID).Error; err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got.Enabled {
		t.Fatal("创建时 enabled=false 被 gorm default 覆盖成了 true")
	}
}
