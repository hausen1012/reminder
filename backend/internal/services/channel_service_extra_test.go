package services

import (
	"testing"
	"time"

	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// newChannelSvcForTest 构造通道服务。
func newChannelSvcForTest(t *testing.T) (*ChannelService, *gorm.DB) {
	t.Helper()
	db := openTestDB(t)
	box, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	return NewChannelService(db, box), db
}

// 各通道类型的必填字段校验。
func TestChannelCreate_RequiresTypeFields(t *testing.T) {
	svc, _ := newChannelSvcForTest(t)

	cases := []struct {
		name string
		in   ChannelInput
	}{
		{"smtp 缺 host", ChannelInput{Name: "a", Type: "smtp", Config: map[string]any{}}},
		{"dingtalk 缺 webhook", ChannelInput{Name: "b", Type: "dingtalk", Config: map[string]any{}}},
		{"wecom 缺 webhook", ChannelInput{Name: "c", Type: "wecom", Config: map[string]any{}}},
		{"webhook 缺 url", ChannelInput{Name: "d", Type: "webhook", Config: map[string]any{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Create(tc.in); err == nil {
				t.Fatalf("期望 %s 校验失败", tc.name)
			}
		})
	}
}

// log 类型无需任何配置即可创建。
func TestChannelCreate_LogTypeNoConfig(t *testing.T) {
	svc, _ := newChannelSvcForTest(t)

	v, err := svc.Create(ChannelInput{Name: "日志", Type: "log", Config: map[string]any{}})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if !v.Enabled {
		t.Fatal("默认应启用")
	}
}

// 敏感字段应以 *** 形式返回，且落库时被加密。
func TestChannelSensitiveFieldsMaskedAndEncrypted(t *testing.T) {
	svc, db := newChannelSvcForTest(t)

	const secret = "my-smtp-password"
	v, err := svc.Create(ChannelInput{
		Name: "smtp",
		Type: "smtp",
		Config: map[string]any{
			"host":         "smtp.test",
			"port":         587,
			"username":     "u@test",
			"password_enc": secret,
			"from_addr":    "f@test",
			"to":           []string{"t@test"},
		},
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 视图里应脱敏
	if got := v.Config["password_enc"]; got != sensitivePlaceholder {
		t.Fatalf("password_enc 应脱敏为 %q，实际 %v", sensitivePlaceholder, got)
	}

	// 数据库中应是密文（不等于明文）
	var raw models.Channel
	if err := db.First(&raw, v.ID).Error; err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got := string(raw.Config); containsSub(got, secret) {
		t.Fatalf("数据库中存在明文密码: %s", got)
	}

	// DecryptedConfig 应能还原明文
	plain, err := svc.DecryptedConfig(&raw)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if !containsSub(string(plain), secret) {
		t.Fatalf("解密后应包含原始密码: %s", plain)
	}
}

// 更新时传 *** 表示不修改，传新值则覆盖。
func TestChannelUpdate_SensitiveFieldPlaceholder(t *testing.T) {
	svc, _ := newChannelSvcForTest(t)

	created, err := svc.Create(ChannelInput{
		Name: "smtp2",
		Type: "smtp",
		Config: map[string]any{
			"host": "smtp.test", "port": 587, "username": "u",
			"password_enc": "orig-pass", "from_addr": "f@test",
			"to": []string{"t@test"},
		},
	})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 传占位符：密码保持原值
	v, err := svc.Update(created.ID, ChannelInput{
		Name: "smtp2",
		Config: map[string]any{
			"host": "smtp2.test", "password_enc": sensitivePlaceholder,
		},
	})
	if err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	if v.Config["host"] != "smtp2.test" {
		t.Fatalf("host 应被更新，实际 %v", v.Config["host"])
	}

	// 传新密码：应覆盖
	if _, err := svc.Update(created.ID, ChannelInput{
		Name:   "smtp2",
		Config: map[string]any{"password_enc": "new-pass"},
	}); err != nil {
		t.Fatalf("更新密码失败: %v", err)
	}
}

// 列表 / 分页 / 过滤 / 排序。
func TestChannelListAndPaged(t *testing.T) {
	svc, _ := newChannelSvcForTest(t)

	for i := 0; i < 3; i++ {
		if _, err := svc.Create(ChannelInput{Name: "ch" + string(rune('A'+i)), Type: "log", Config: map[string]any{}}); err != nil {
			t.Fatalf("创建失败: %v", err)
		}
	}

	all, err := svc.List()
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("应为 3 条，实际 %d", len(all))
	}

	rows, total, err := svc.ListPaged(ChannelListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("ListPaged 失败: %v", err)
	}
	if total != 3 || len(rows) != 2 {
		t.Fatalf("分页应为 total=3 len=2，实际 total=%d len=%d", total, len(rows))
	}

	// 搜索
	rows, total, err = svc.ListPaged(ChannelListFilter{Limit: 10, Search: "chA"})
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("搜索应命中 1 条，实际 %d", total)
	}

	// enabled 过滤
	enabled := false
	_, total, err = svc.ListPaged(ChannelListFilter{Limit: 10, Enabled: &enabled})
	if err != nil {
		t.Fatalf("过滤失败: %v", err)
	}
	if total != 0 {
		t.Fatalf("enabled=false 应为 0，实际 %d", total)
	}

	// 排序白名单
	if _, _, err := svc.ListPaged(ChannelListFilter{Limit: 10, SortBy: "name; DROP TABLE", SortOrder: "asc"}); err != nil {
		t.Fatalf("非法排序应回退而非报错: %v", err)
	}
}

// Toggle：切换启用状态。
func TestChannelToggle(t *testing.T) {
	svc, _ := newChannelSvcForTest(t)

	created, err := svc.Create(ChannelInput{Name: "toggle", Type: "log", Config: map[string]any{}})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	v, err := svc.Toggle(created.ID)
	if err != nil {
		t.Fatalf("切换失败: %v", err)
	}
	if v.Enabled {
		t.Fatal("应变为禁用")
	}
}

// Delete 与 BatchDelete。
func TestChannelDelete(t *testing.T) {
	svc, _ := newChannelSvcForTest(t)

	c1, err := svc.Create(ChannelInput{Name: "d1", Type: "log", Config: map[string]any{}})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	c2, err := svc.Create(ChannelInput{Name: "d2", Type: "log", Config: map[string]any{}})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	if err := svc.Delete(c1.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, err := svc.Get(c1.ID); err == nil {
		t.Fatal("删除后应查不到")
	}

	if err := svc.BatchDelete([]uint{c2.ID}); err != nil {
		t.Fatalf("批量删除失败: %v", err)
	}
	if all, _ := svc.List(); len(all) != 0 {
		t.Fatalf("应全部删除，实际 %d", len(all))
	}

	// 空 ids 不应报错
	if err := svc.BatchDelete(nil); err != nil {
		t.Fatalf("空 ids 不应报错: %v", err)
	}
}

// Stats 应按窗口统计各通道的投递情况。
func TestChannelStats(t *testing.T) {
	svc, db := newChannelSvcForTest(t)

	ch, err := svc.Create(ChannelInput{Name: "stat", Type: "log", Config: map[string]any{}})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 造投递日志与 attempt
	r := &models.Reminder{Title: "r", Calendar: "solar", ScheduleType: "once", Enabled: true}
	if err := db.Create(r).Error; err != nil {
		t.Fatalf("创建提醒失败: %v", err)
	}
	dl := &models.DeliveryLog{ReminderID: r.ID, FiredAt: time.Now(), Title: "t", Status: "success", Source: "web"}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("创建日志失败: %v", err)
	}
	db.Create(&models.DeliveryAttempt{
		DeliveryLogID: dl.ID, ChannelID: ch.ID, ChannelType: "log",
		ChannelName: "stat", Attempt: 1, Status: "success",
	})

	stats, err := svc.Stats(24 * time.Hour)
	if err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if len(stats) == 0 {
		t.Fatal("应至少返回一条统计")
	}
	if stats[0].Total < 1 {
		t.Fatalf("total = %d，期望 >= 1", stats[0].Total)
	}
}

// DryRun 对未知类型应报错，对 log 类型应成功。
func TestChannelDryRun(t *testing.T) {
	svc, _ := newChannelSvcForTest(t)

	if err := svc.DryRun(t.Context(), "unknown", map[string]any{}); err == nil {
		t.Fatal("未知类型应报错")
	}
	if err := svc.DryRun(t.Context(), "log", map[string]any{}); err != nil {
		t.Fatalf("log 类型试发应成功: %v", err)
	}
}

// 名称/类型/配置的基础校验。
func TestChannelValidateBasics(t *testing.T) {
	svc, _ := newChannelSvcForTest(t)

	cases := []ChannelInput{
		{Name: "", Type: "log", Config: map[string]any{}},                       // 名称空
		{Name: string(make([]rune, 65)), Type: "log", Config: map[string]any{}}, // 名称超长
		{Name: "x", Type: "nope", Config: map[string]any{}},                     // 未知类型
		{Name: "x", Type: "log", Config: nil},                                   // config 空
	}
	for i, in := range cases {
		if _, err := svc.Create(in); err == nil {
			t.Fatalf("第 %d 种非法输入应被拒绝", i)
		}
	}
}

// chunkIDs 应正确分批。
func TestChunkIDs(t *testing.T) {
	if got := chunkIDs(nil, 10); got != nil {
		t.Fatalf("空输入应返回 nil，实际 %v", got)
	}

	ids := []uint{1, 2, 3, 4, 5}
	got := chunkIDs(ids, 2)
	if len(got) != 3 {
		t.Fatalf("应分 3 批，实际 %d", len(got))
	}
	if len(got[0]) != 2 || len(got[2]) != 1 {
		t.Fatalf("分批大小不符: %v", got)
	}

	// size 大于总数时一批
	if got := chunkIDs(ids, 100); len(got) != 1 {
		t.Fatalf("应分 1 批，实际 %d", len(got))
	}
}

// containsSub 判断字符串包含关系。
func containsSub(s, sub string) bool {
	return len(sub) == 0 || indexOfSub(s, sub) >= 0
}

func indexOfSub(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
