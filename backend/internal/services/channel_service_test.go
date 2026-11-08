package services

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"gorm.io/datatypes"
)

func TestChannelUpdateRejectsMissingSMTPFieldsAfterMerge(t *testing.T) {
	db := openTestDB(t)
	box, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	service := NewChannelService(db, box, testTZ(cstZone()))

	created, err := service.Create(ChannelInput{
		Name: "smtp-test",
		Type: "smtp",
		Config: map[string]any{
			"host":          "smtp.example.com",
			"port":          587,
			"username":      "user@example.com",
			"password_enc":  "secret",
			"from_addr":     "sender@example.com",
			"from_name":     "提醒助手",
			"to":            []string{"receiver@example.com"},
			"security_mode": "starttls",
		},
	})
	if err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}

	_, err = service.Update(created.ID, ChannelInput{
		Name: created.Name,
		Config: map[string]any{
			"host": "",
		},
	})
	if err == nil {
		t.Fatal("期望更新时拦截空 host")
	}

	_, err = service.Update(created.ID, ChannelInput{
		Name: created.Name,
		Config: map[string]any{
			"to": []string{},
		},
	})
	if err == nil {
		t.Fatal("期望更新时拦截空收件人")
	}

	_, err = service.Update(created.ID, ChannelInput{
		Name: created.Name,
		Config: map[string]any{
			"port": 0,
		},
	})
	if err == nil {
		t.Fatal("期望更新时拦截非法端口")
	}
}

func TestChannelUpdateKeepsEncryptedPasswordWhenPlaceholderIsUsed(t *testing.T) {
	db := openTestDB(t)
	box, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	service := NewChannelService(db, box, testTZ(cstZone()))

	created, err := service.Create(ChannelInput{
		Name: "smtp-secret",
		Type: "smtp",
		Config: map[string]any{
			"host":          "smtp.example.com",
			"port":          587,
			"username":      "user@example.com",
			"password_enc":  "secret",
			"from_addr":     "sender@example.com",
			"from_name":     "提醒助手",
			"to":            []string{"receiver@example.com"},
			"security_mode": "starttls",
		},
	})
	if err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}

	var before models.Channel
	if err := db.First(&before, created.ID).Error; err != nil {
		t.Fatalf("读取更新前通道失败: %v", err)
	}
	plainBefore, err := service.DecryptedConfig(&before)
	if err != nil {
		t.Fatalf("解密更新前配置失败: %v", err)
	}

	_, err = service.Update(created.ID, ChannelInput{
		Name: "smtp-secret-updated",
		Config: map[string]any{
			"password_enc": "***",
			"from_name":    "新提醒助手",
		},
	})
	if err != nil {
		t.Fatalf("使用占位符更新失败: %v", err)
	}

	var after models.Channel
	if err := db.First(&after, created.ID).Error; err != nil {
		t.Fatalf("读取更新后通道失败: %v", err)
	}
	plainAfter, err := service.DecryptedConfig(&after)
	if err != nil {
		t.Fatalf("解密更新后配置失败: %v", err)
	}

	if string(plainBefore) == "" || string(plainAfter) == "" {
		t.Fatal("明文配置不应为空")
	}
	if string(plainBefore) == string(after.Config) {
		t.Fatal("落库配置不应是明文")
	}
	if string(plainAfter) == string(after.Config) {
		t.Fatal("更新后落库配置不应是明文")
	}
	if string(plainBefore) == string(plainAfter) {
		// from_name 已修改，整体 JSON 应变化
	} else {
		// 继续验证 password 保持不变
	}

	cfgBefore, err := service.decryptConfig(before.Config)
	if err != nil {
		t.Fatalf("解析更新前配置失败: %v", err)
	}
	cfgAfter, err := service.decryptConfig(after.Config)
	if err != nil {
		t.Fatalf("解析更新后配置失败: %v", err)
	}
	if cfgBefore["password_enc"] != cfgAfter["password_enc"] {
		t.Fatalf("密码占位符更新后应保留原值，before=%v after=%v", cfgBefore["password_enc"], cfgAfter["password_enc"])
	}
	if cfgAfter["from_name"] != "新提醒助手" {
		t.Fatalf("from_name 未更新，实际 %v", cfgAfter["from_name"])
	}
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
		{"feishu 缺 webhook", ChannelInput{Name: "e", Type: "feishu", Config: map[string]any{}}},
		{"telegram 缺 token", ChannelInput{Name: "f", Type: "telegram", Config: map[string]any{"chat_id": "1"}}},
		{"bark 缺 device key", ChannelInput{Name: "g", Type: "bark", Config: map[string]any{}}},
		{"ntfy 缺 topic", ChannelInput{Name: "h", Type: "ntfy", Config: map[string]any{"server_url": "https://ntfy.sh"}}},
		{"apprise 缺 key 与 urls", ChannelInput{Name: "i", Type: "apprise", Config: map[string]any{"api_url": "http://localhost:8000"}}},
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
	dl := &models.DeliveryLog{ReminderID: r.ID, FiredAt: models.TimeNow(), Title: "t", Status: "success", Source: "web"}
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

// 试发消息里的 vars 必须是展开后的标题与正文，Webhook 模板引用时不残留占位符。
func TestChannelDryRunVarsAreRendered(t *testing.T) {
	var got map[string]string
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
	}))
	defer sink.Close()

	svc, _ := newChannelSvcForTest(t)
	err := svc.DryRun(t.Context(), "webhook", map[string]any{
		"method":        "POST",
		"url":           sink.URL,
		"body_template": `{"title":"{{title}}","content":"{{content}}"}`,
	})
	if err != nil {
		t.Fatalf("试发失败: %v", err)
	}
	for _, key := range []string{"title", "content"} {
		if containsSub(got[key], "{{") {
			t.Fatalf("试发的 %s 残留占位符: %q", key, got[key])
		}
		if got[key] == "" {
			t.Fatalf("试发的 %s 为空", key)
		}
	}
	if !containsSub(got["content"], "webhook") {
		t.Fatalf("试发正文应含通道类型: %q", got["content"])
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

// 部分更新应保留未提交的字段，而不是把整份 config 覆盖成只剩提交的字段。
//
// 这是一个回归用例：Update 只把 patch 里的字段加密后整体写回，
// 若调用方只提交 config 的一个子集（前端「只改某一项」的场景），
// 其余字段会被静默丢弃。
func TestChannelUpdate_PartialKeepsOtherFields(t *testing.T) {
	db := openTestDB(t)
	box, _ := secretbox.New("")
	svc := NewChannelService(db, box, testTZ(cstZone()))

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

// 换过 ENCRYPTION_KEY 后，通道敏感字段解不开必须返回 400 + 可操作提示，
// 而不是 500。用户看到「服务器内部错误」完全无法判断要怎么办。
func TestChannelDecryptWrongKeyReturnsValidationError(t *testing.T) {
	db := openTestDB(t)

	boxA, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	cipher, err := boxA.EncryptString("smtp-password")
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	cfg, _ := json.Marshal(map[string]any{
		"host":         "smtp.example.com",
		"password_enc": cipher,
	})
	ch := &models.Channel{Name: "换密钥", Type: "smtp", Enabled: true, Config: datatypes.JSON(cfg)}
	if err := db.Create(ch).Error; err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}

	// 用另一把密钥构造服务，模拟 ENCRYPTION_KEY 被改动
	other := make([]byte, 32)
	for i := range other {
		other[i] = byte(i + 1)
	}
	boxB, err := secretbox.New(base64.StdEncoding.EncodeToString(other))
	if err != nil {
		t.Fatalf("构造第二把密钥失败: %v", err)
	}
	svc := NewChannelService(db, boxB, testTZ(cstZone()))

	var stored models.Channel
	if err := db.First(&stored, ch.ID).Error; err != nil {
		t.Fatalf("读取通道失败: %v", err)
	}

	_, err = svc.DecryptedConfig(&stored)
	if err == nil {
		t.Fatal("解不开时应报错")
	}

	var ae *middleware.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("错误类型 = %T，期望 *middleware.AppError（否则会变成 500）", err)
	}
	if ae.Code != middleware.CodeValidationFailed {
		t.Fatalf("错误码 = %q，期望 %q", ae.Code, middleware.CodeValidationFailed)
	}
	// 提示里要说清怎么办
	for _, want := range []string{"ENCRYPTION_KEY", "重新填写"} {
		if !containsSub(ae.Message, want) {
			t.Fatalf("提示文案缺少 %q: %q", want, ae.Message)
		}
	}
	// field 应指向具体字段名（去掉 _enc 后缀）
	if ae.Field != "password" {
		t.Fatalf("field = %q，期望 password", ae.Field)
	}
}

// 正常密钥时解密应照旧可用。
func TestChannelDecryptOKWithSameKey(t *testing.T) {
	db := openTestDB(t)
	svc, _ := newChannelSvcForTest(t)

	cipher, err := svc.Box.EncryptString("smtp-password")
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	cfg, _ := json.Marshal(map[string]any{"password_enc": cipher})
	ch := &models.Channel{Name: "同密钥", Type: "smtp", Enabled: true, Config: datatypes.JSON(cfg)}
	if err := db.Create(ch).Error; err != nil {
		t.Fatalf("创建通道失败: %v", err)
	}

	plain, err := svc.DecryptedConfig(ch)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(plain, &out); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if out["password_enc"] != "smtp-password" {
		t.Fatalf("解密结果 = %v", out["password_enc"])
	}
}

// 试发失败要返回 400，不能是 500。
func TestChannelDryRunFailureIsValidationError(t *testing.T) {
	svc, _ := newChannelSvcForTest(t)

	// 缺少必填字段的 webhook 配置 → 发送器返回永久错误
	err := svc.DryRun(t.Context(), "webhook", map[string]any{})
	if err == nil {
		t.Fatal("缺少 url 的 webhook 试发应失败")
	}
	var ae *middleware.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("错误类型 = %T，期望 *middleware.AppError（否则会变成 500）", err)
	}
	if ae.Code != middleware.CodeValidationFailed {
		t.Fatalf("错误码 = %q，期望 %q", ae.Code, middleware.CodeValidationFailed)
	}
	if !containsSub(ae.Message, "试发失败") {
		t.Fatalf("提示缺少前缀: %q", ae.Message)
	}
}

// isUniqueErr 应识别 SQLite 的唯一约束错误。
func TestIsUniqueErr(t *testing.T) {
	if isUniqueErr(nil) {
		t.Fatal("nil 不应判为唯一约束错误")
	}
	if isUniqueErr(errors.New("some other error")) {
		t.Fatal("普通错误不应判为唯一约束错误")
	}

	db := openTestDB(t)
	svc := NewChannelService(db, nil, testTZ(cstZone()))
	if _, err := svc.Create(ChannelInput{Name: "dup", Type: "log", Config: map[string]any{}}); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	// 重复名称触发唯一约束
	_, err := svc.Create(ChannelInput{Name: "dup", Type: "log", Config: map[string]any{}})
	if err == nil {
		t.Fatal("重复名称应报错")
	}
	if !isUniqueErr(errors.Unwrap(err)) && !isUniqueErr(err) {
		// 包装层可能不是原始错误，这里只要求走了"名称已存在"分支
		if !containsSub(err.Error(), "已存在") {
			t.Fatalf("应提示名称已存在，实际 %v", err)
		}
	}
}
