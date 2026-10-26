package scheduler

import (
	"testing"

	"github.com/reminder/backend/internal/models"
	"gorm.io/datatypes"
)

// 回归测试：Enabled 字段带 gorm default:true，若 Create 时传 false，
// GORM 会用默认值覆盖，导致「创建时禁用」静默变成「启用」。
func TestReminderCreateWithDisabledIsNotOverridden(t *testing.T) {
	db := openSchedulerTestDB(t)

	r := &models.Reminder{
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

	var got models.Reminder
	if err := db.First(&got, r.ID).Error; err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got.Enabled {
		t.Fatal("创建时 enabled=false 被 gorm default 覆盖成了 true")
	}
}
