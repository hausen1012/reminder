package services

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/reminder/backend/internal/crypto/secretbox"
	"github.com/reminder/backend/internal/middleware"
	"github.com/reminder/backend/internal/models"
	"gorm.io/gorm"
)

// newTokenSvc 构造令牌服务。
func newTokenSvc(t *testing.T) (*TokenService, *gorm.DB) {
	t.Helper()
	db := openTestDB(t)
	box, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	return NewTokenService(db, box), db
}

// 创建的令牌应带 bdrk_ 前缀，且可用明文通过校验。
func TestTokenCreateAndVerify(t *testing.T) {
	svc, _ := newTokenSvc(t)

	plain, key, err := svc.Create("测试", nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if !strings.HasPrefix(plain, keyPrefix) {
		t.Fatalf("明文应以 %s 开头，实际 %q", keyPrefix, plain)
	}
	if key.Prefix != plain[:len(keyPrefix)+8] {
		t.Fatalf("prefix = %q 与明文前缀不符", key.Prefix)
	}

	got, ok := svc.Verify(plain)
	if !ok {
		t.Fatal("明文应能通过校验")
	}
	if got.ID != key.ID {
		t.Fatalf("校验返回的 ID = %d，期望 %d", got.ID, key.ID)
	}
}

// 校验应拒绝：前缀错误、随机串、已禁用令牌。
func TestTokenVerify_Rejects(t *testing.T) {
	svc, _ := newTokenSvc(t)

	plain, key, err := svc.Create("待测", nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	if _, ok := svc.Verify("wrong-prefix"); ok {
		t.Fatal("前缀错误应拒绝")
	}
	if _, ok := svc.Verify(keyPrefix + "nonexistent"); ok {
		t.Fatal("不存在的令牌应拒绝")
	}

	// 禁用后应拒绝
	if _, err := svc.Toggle(key.ID); err != nil {
		t.Fatalf("禁用失败: %v", err)
	}
	if _, ok := svc.Verify(plain); ok {
		t.Fatal("已禁用的令牌应拒绝")
	}
}

// 名称必填。
func TestTokenCreate_EmptyName(t *testing.T) {
	svc, _ := newTokenSvc(t)

	if _, _, err := svc.Create("  ", nil); err == nil {
		t.Fatal("空名称应报错")
	}
}

// 列表支持分页与搜索。
func TestTokenList(t *testing.T) {
	svc, _ := newTokenSvc(t)

	for _, name := range []string{"甲", "乙", "丙"} {
		if _, _, err := svc.Create(name, nil); err != nil {
			t.Fatalf("创建 %s 失败: %v", name, err)
		}
	}

	rows, total, err := svc.List(10, 0, "")
	if err != nil {
		t.Fatalf("列表失败: %v", err)
	}
	if total != 3 || len(rows) != 3 {
		t.Fatalf("应为 3 条，实际 total=%d len=%d", total, len(rows))
	}

	rows, total, err = svc.List(2, 0, "")
	if err != nil {
		t.Fatalf("分页失败: %v", err)
	}
	if total != 3 || len(rows) != 2 {
		t.Fatalf("分页应为 total=3 len=2，实际 total=%d len=%d", total, len(rows))
	}

	rows, total, err = svc.List(10, 0, "乙")
	if err != nil {
		t.Fatalf("搜索失败: %v", err)
	}
	if total != 1 || rows[0].Name != "乙" {
		t.Fatalf("搜索应命中 1 条，实际 total=%d", total)
	}
}

// Toggle 应来回切换；Delete 后查不到。
func TestTokenToggleAndDelete(t *testing.T) {
	svc, _ := newTokenSvc(t)

	_, key, err := svc.Create("切换", nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	got, err := svc.Toggle(key.ID)
	if err != nil {
		t.Fatalf("禁用失败: %v", err)
	}
	if got.Enabled {
		t.Fatal("应变为禁用")
	}

	got, err = svc.Toggle(key.ID)
	if err != nil {
		t.Fatalf("启用失败: %v", err)
	}
	if !got.Enabled {
		t.Fatal("应变为启用")
	}

	if err := svc.Delete(key.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if _, err := svc.GetView(key.ID); err == nil {
		t.Fatal("删除后应查不到")
	}
}

// 对不存在的 ID 操作应返回 not_found。
func TestTokenNotFound(t *testing.T) {
	svc, _ := newTokenSvc(t)

	if _, err := svc.Toggle(9999); err == nil {
		t.Fatal("Toggle 不存在应报错")
	}
	if err := svc.Delete(9999); err == nil {
		t.Fatal("Delete 不存在应报错")
	}
	if _, err := svc.GetView(9999); err == nil {
		t.Fatal("GetView 不存在应报错")
	}
	if _, err := svc.GetPlaintext(9999); err == nil {
		t.Fatal("GetPlaintext 不存在应报错")
	}
}

// 默认通道：创建时绑定、更新时替换、去重。
func TestTokenDefaultChannels(t *testing.T) {
	svc, _ := newTokenSvc(t)

	_, key, err := svc.Create("带通道", []uint{1, 2, 2}) // 重复 ID 应去重
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	ids := svc.DefaultChannelIDs(key.ID)
	if len(ids) != 2 {
		t.Fatalf("应绑定 2 个通道，实际 %v", ids)
	}

	if err := svc.UpdateDefaultChannels(key.ID, []uint{3}); err != nil {
		t.Fatalf("更新通道失败: %v", err)
	}
	ids = svc.DefaultChannelIDs(key.ID)
	if len(ids) != 1 || ids[0] != 3 {
		t.Fatalf("应替换为 [3]，实际 %v", ids)
	}

	// 清空
	if err := svc.UpdateDefaultChannels(key.ID, nil); err != nil {
		t.Fatalf("清空通道失败: %v", err)
	}
	if ids = svc.DefaultChannelIDs(key.ID); len(ids) != 0 {
		t.Fatalf("应已清空，实际 %v", ids)
	}
}

// Stats24h 只统计 24 小时内该令牌创建的提醒。
func TestTokenStats24h(t *testing.T) {
	svc, db := newTokenSvc(t)

	_, key, err := svc.Create("统计", nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 该令牌的两条提醒
	for i := 0; i < 2; i++ {
		r := &models.Reminder{Title: "r", Calendar: "solar", ScheduleType: "once", Enabled: true, TokenID: &key.ID}
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("创建提醒失败: %v", err)
		}
	}
	// 其他令牌的提醒（不应计入）
	otherID := uint(999)
	db.Create(&models.Reminder{Title: "o", Calendar: "solar", ScheduleType: "once", Enabled: true, TokenID: &otherID})

	if got := svc.Stats24h(key.ID); got != 2 {
		t.Fatalf("Stats24h = %d，期望 2", got)
	}
}

// GetPlaintext：有明文返回；明文为空时返回 not_found。
func TestTokenGetPlaintext(t *testing.T) {
	svc, db := newTokenSvc(t)

	plain, key, err := svc.Create("明文", nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	got, err := svc.GetPlaintext(key.ID)
	if err != nil {
		t.Fatalf("读取明文失败: %v", err)
	}
	if got != plain {
		t.Fatalf("明文不一致: %q vs %q", got, plain)
	}

	// 明文被清空后应报错
	if err := db.Exec("UPDATE tokens SET plaintext = '' WHERE id = ?", key.ID).Error; err != nil {
		t.Fatalf("清空明文失败: %v", err)
	}
	if _, err := svc.GetPlaintext(key.ID); err == nil {
		t.Fatal("明文为空应报错")
	}
}

// ListViews / GetView 应带出默认通道与 24h 用量。
func TestTokenViews(t *testing.T) {
	svc, db := newTokenSvc(t)

	_, key, err := svc.Create("视图", []uint{7, 8})
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	_, other, err := svc.Create("另一个", nil)
	if err != nil {
		t.Fatalf("创建第二个失败: %v", err)
	}
	// 只有第一个令牌有 24h 内的提醒
	for i := 0; i < 3; i++ {
		r := &models.Reminder{Title: "r", Calendar: "solar", ScheduleType: "once", Enabled: true, TokenID: &key.ID}
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("创建提醒失败: %v", err)
		}
	}

	views, total, err := svc.ListViews(10, 0, "")
	if err != nil {
		t.Fatalf("ListViews 失败: %v", err)
	}
	if total != 2 || len(views) != 2 {
		t.Fatalf("应为 2 条，实际 total=%d", total)
	}
	byID := map[uint]*TokenView{}
	for _, v := range views {
		byID[v.ID] = v
	}
	if got := byID[key.ID].DefaultChannelIDs; len(got) != 2 {
		t.Fatalf("应带出 2 个默认通道，实际 %v", got)
	}
	if got := byID[key.ID].Usage24h; got != 3 {
		t.Fatalf("24h 用量 = %d，期望 3", got)
	}
	if got := byID[other.ID].Usage24h; got != 0 {
		t.Fatalf("无提醒的令牌用量应为 0，实际 %d", got)
	}
	// 无默认通道时应是空切片而不是 nil（前端直接 .length）
	if byID[other.ID].DefaultChannelIDs == nil {
		t.Fatal("无默认通道时应返回空切片")
	}

	view, err := svc.GetView(key.ID)
	if err != nil {
		t.Fatalf("GetView 失败: %v", err)
	}
	if view.Name != "视图" {
		t.Fatalf("name = %q", view.Name)
	}
	if view.Usage24h != 3 {
		t.Fatalf("GetView 24h 用量 = %d，期望 3", view.Usage24h)
	}
}

// TouchLastUsed 应节流：短时间内多次调用只写一次。
func TestTokenTouchLastUsed_Throttled(t *testing.T) {
	svc, db := newTokenSvc(t)

	_, key, err := svc.Create("touch", nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 连续调用 5 次，内部应只发一次写库 goroutine
	for i := 0; i < 5; i++ {
		svc.TouchLastUsed(key.ID)
	}

	// goroutine 异步写库，给它一点时间；主要验证不 panic、不重复写
	var got models.Token
	if err := db.First(&got, key.ID).Error; err != nil {
		t.Fatalf("读取失败: %v", err)
	}
}

// generateKey 每次应生成不同且长度稳定的令牌。
func TestGenerateKey(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		k, err := generateKey()
		if err != nil {
			t.Fatalf("生成失败: %v", err)
		}
		if !strings.HasPrefix(k, keyPrefix) {
			t.Fatalf("缺少前缀: %q", k)
		}
		if seen[k] {
			t.Fatalf("生成了重复令牌: %q", k)
		}
		seen[k] = true
	}
}

// base62Encode 应只产出 base62 字符集。
func TestBase62Encode(t *testing.T) {
	cases := [][]byte{
		{0},
		{1},
		{255},
		{0, 0, 0},
		[]byte("hello world, this is a longer input 1234567890"),
	}
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	for _, in := range cases {
		out := base62Encode(in)
		if out == "" {
			t.Fatalf("输入 %v 输出为空", in)
		}
		for _, ch := range out {
			if !strings.ContainsRune(charset, ch) {
				t.Fatalf("输出含非 base62 字符 %q: %s", ch, out)
			}
		}
	}
}

// sha256Hex 应稳定且为 64 位 hex。
func TestSha256Hex(t *testing.T) {
	a := sha256Hex("abc")
	b := sha256Hex("abc")
	if a != b {
		t.Fatal("相同输入应得到相同哈希")
	}
	if len(a) != 64 {
		t.Fatalf("长度 = %d，期望 64", len(a))
	}
	if sha256Hex("abd") == a {
		t.Fatal("不同输入应得到不同哈希")
	}
}

// 令牌明文必须以密文入库，不能出现可直接使用的 bdrk_ 明文。
func TestTokenPlaintextStoredEncrypted(t *testing.T) {
	svc, db := newTokenSvc(t)

	plain, key, err := svc.Create("加密存储", nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	var stored models.Token
	if err := db.First(&stored, key.ID).Error; err != nil {
		t.Fatalf("读取令牌失败: %v", err)
	}
	if stored.Plaintext == plain {
		t.Fatalf("明文被直接入库: %q", stored.Plaintext)
	}
	if strings.HasPrefix(stored.Plaintext, keyPrefix) {
		t.Fatalf("库里仍能看到令牌前缀，说明是明文存储: %q", stored.Plaintext)
	}
	if stored.Plaintext == "" {
		t.Fatal("明文列不应为空")
	}

	// 但仍然能取回原文
	got, err := svc.GetPlaintext(key.ID)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if got != plain {
		t.Fatalf("解密结果 = %q，期望 %q", got, plain)
	}

	// 鉴权用的哈希不受影响
	if _, ok := svc.Verify(plain); !ok {
		t.Fatal("加密存储后明文仍应能通过校验")
	}
}

// 没有配置 Box（仅测试场景）时应原样存取，不应崩。
func TestTokenPlaintextWithoutBox(t *testing.T) {
	db := openTestDB(t)
	svc := NewTokenService(db, nil)

	plain, key, err := svc.Create("无密钥", nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	got, err := svc.GetPlaintext(key.ID)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got != plain {
		t.Fatalf("明文 = %q，期望 %q", got, plain)
	}
}

// 换密钥后解不开时应返回 404 + 可操作提示，而不是 500，也不能把密文当明文返回。
func TestTokenPlaintextWithWrongKeyReturnsNotFound(t *testing.T) {
	db := openTestDB(t)

	boxA, err := secretbox.New("")
	if err != nil {
		t.Fatalf("构造 secretbox 失败: %v", err)
	}
	plain, key, err := NewTokenService(db, boxA).Create("换密钥", nil)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 换用另一把密钥构造服务，模拟 ENCRYPTION_KEY 被改动
	other := make([]byte, 32)
	for i := range other {
		other[i] = byte(i + 1)
	}
	boxB, err := secretbox.New(base64.StdEncoding.EncodeToString(other))
	if err != nil {
		t.Fatalf("构造第二把密钥失败: %v", err)
	}

	got, err := NewTokenService(db, boxB).GetPlaintext(key.ID)
	if err == nil {
		t.Fatalf("解不开时应报错，实际返回 %q", got)
	}

	// 必须是 not_found（404），不能是 internal_error（500）
	var ae *middleware.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("错误类型 = %T，期望 *middleware.AppError", err)
	}
	if ae.Code != middleware.CodeNotFound {
		t.Fatalf("错误码 = %q，期望 %q（不能是 500）", ae.Code, middleware.CodeNotFound)
	}
	if ae.Message == "" {
		t.Fatal("应给出可操作的提示文案")
	}

	// 关键：不能把库里的密文当明文返回，否则用户会误以为那是令牌
	if got == plain {
		t.Fatal("换密钥后不应还能拿到原明文")
	}
	if got != "" {
		t.Fatalf("出错时不应返回任何内容，实际 %q", got)
	}

	// 令牌本身照旧可用：鉴权走独立保存的 sha256 哈希
	if _, ok := NewTokenService(db, boxB).Verify(plain); !ok {
		t.Fatal("换密钥不应影响令牌鉴权")
	}
}

// ListViews 的查询数不应随令牌数量增长（防 N+1）。
//
// 5 个令牌时固定为 5 次查询：count + 分页 + 默认通道批量 + 用量批量，
// 逐条查会变成 1+2N 次。
func TestTokenListViews_QueryCountIndependentOfRowCount(t *testing.T) {
	svc, db := newTokenSvc(t)

	for i := 0; i < 5; i++ {
		if _, _, err := svc.Create("n-plus-one-"+itoaTest(i), []uint{uint(i + 1)}); err != nil {
			t.Fatalf("创建失败: %v", err)
		}
	}

	var views []*TokenView
	n := countQueries(t, db, func(scoped *gorm.DB) {
		scopedSvc := NewTokenService(scoped, nil)
		items, total, err := scopedSvc.ListViews(10, 0, "")
		if err != nil {
			t.Fatalf("ListViews 失败: %v", err)
		}
		if total != 5 || len(items) != 5 {
			t.Fatalf("应返回 5 条，实际 total=%d len=%d", total, len(items))
		}
		views = items
	})
	if n > 5 {
		t.Fatalf("ListViews 产生了 %d 次查询，期望 ≤5", n)
	}
	for _, v := range views {
		if len(v.DefaultChannelIDs) != 1 {
			t.Fatalf("令牌 %d 的默认通道数 = %d，期望 1", v.ID, len(v.DefaultChannelIDs))
		}
	}
}

// 验证：删除令牌时级联清理 token_default_channels 中的绑定关系，避免通道后续因孤儿关联无法被删除。
func TestTokenDelete_CleansUpDefaultChannels(t *testing.T) {
	svc, db := newTokenSvc(t)

	_, token, err := svc.Create("带通道令牌", []uint{1, 2})
	if err != nil {
		t.Fatalf("创建令牌失败: %v", err)
	}

	var count int64
	db.Model(&models.TokenDefaultChannel{}).Where("token_id = ?", token.ID).Count(&count)
	if count != 2 {
		t.Fatalf("创建后应有 2 条默认通道关联，实际 %d", count)
	}

	if err := svc.Delete(token.ID); err != nil {
		t.Fatalf("删除令牌失败: %v", err)
	}

	db.Model(&models.TokenDefaultChannel{}).Where("token_id = ?", token.ID).Count(&count)
	if count != 0 {
		t.Fatalf("删除令牌后默认通道关联应被清空，实际残留 %d 条", count)
	}
}
