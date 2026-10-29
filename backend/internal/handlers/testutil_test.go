// 本包测试脚手架：内存库、请求辅助、造数辅助、固定时区。
//
// 合并自 testutil_test.go / helper_test.go / seed_test.go / user_seed_test.go /
// timezone_testutil_test.go，供本包全部测试文件共用。
package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/models"
	"github.com/reminder/backend/internal/testutil"
	"github.com/reminder/backend/internal/timezone"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// newTestDB 构造一个仅供测试使用的内存 SQLite 数据库。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.NewDB(t)
}

// testTZ 把固定时区包装成 timezone.Provider，供测试注入。
func testTZ(loc *time.Location) *timezone.Provider {
	return testutil.TZ(loc)
}

// cstZone 是测试默认时区（东八区），与生产默认值一致。
func cstZone() *time.Location {
	return testutil.CST()
}

// doJSON 发起一次 JSON 请求（body 为 nil 时不带请求体）。
func doJSON(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	return serve(r, newRequest(method, path, body))
}

// newRequest 构造一次带 JSON 请求头的请求。
func newRequest(method, path string, body any) *http.Request {
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// serve 用指定请求执行引擎并返回响应记录器。
func serve(r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// authed 发起一次带 Bearer 令牌的请求。
func authed(r *gin.Engine, method, path, token string, body any) *httptest.ResponseRecorder {
	req := newRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	return serve(r, req)
}

// dataID 从 `{code,message,data:{id}}` 响应中取出 id。
func dataID(t *testing.T, w *httptest.ResponseRecorder) uint {
	t.Helper()
	var resp struct {
		Data struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	decodeJSON(t, w, &resp)
	if resp.Data.ID == 0 {
		t.Fatalf("响应中缺少 id: %s", w.Body.String())
	}
	return resp.Data.ID
}

// decodeJSON 把响应体解析到 out，失败即测试失败。
func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
		t.Fatalf("响应不是合法 JSON: %v, body=%s", err, w.Body.String())
	}
}

// itoa 是 strconv.Itoa 的简写，避免测试里到处写导入。
func itoa(v uint) string { return strconv.FormatUint(uint64(v), 10) }

// containsKey 判断 JSON 响应体中是否出现某个字段名。
func containsKey(body, key string) bool {
	return strings.Contains(body, `"`+key+`"`)
}

// containsSub 判断字符串是否包含子串。
func containsSub(s, sub string) bool {
	return strings.Contains(s, sub)
}

// seedLog 插入一条投递日志用于测试，返回其 ID。
// retryRound > 0 表示确认链上的重发记录。
func seedLog(t *testing.T, db *gorm.DB, title string, retryRound int, chainID *string) uint {
	t.Helper()
	dl := &models.DeliveryLog{
		ReminderID:     1,
		FiredAt:        models.TimeOf(time.Now().Add(-time.Duration(retryRound) * time.Minute)),
		Title:          title,
		Content:        "正文",
		Status:         "success",
		Source:         "web",
		RetryRound:     retryRound,
		ConfirmChainID: chainID,
	}
	if err := db.Create(dl).Error; err != nil {
		t.Fatalf("插入测试日志失败: %v", err)
	}
	return dl.ID
}

// seedAttempt 为指定日志插入一条投递尝试记录。
func seedAttempt(t *testing.T, db *gorm.DB, deliveryLogID uint, status string) {
	t.Helper()
	a := &models.DeliveryAttempt{
		DeliveryLogID: deliveryLogID,
		ChannelID:     1,
		ChannelType:   "log",
		ChannelName:   fmt.Sprintf("ch-%d", time.Now().UnixNano()),
		Attempt:       1,
		Status:        status,
	}
	if err := db.Create(a).Error; err != nil {
		t.Fatalf("插入测试 attempt 失败: %v", err)
	}
}

// seedUser 创建一个用户名为 admin、密码为 admin123 的测试用户。
func seedUser(t *testing.T, db *gorm.DB) *models.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("生成密码哈希失败: %v", err)
	}
	user := &models.User{Username: "admin", Password: string(hash)}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("创建测试用户失败: %v", err)
	}
	return user
}
