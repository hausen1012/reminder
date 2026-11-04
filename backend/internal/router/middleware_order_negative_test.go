package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/middleware"
)

// 反证：把 CORS 挪到 BodyLimit 之后，413 确实会丢掉跨域头。
//
// 如果不写这条，TestMiddlewareOrder_* 可能因为「CORS 头本来就会加」而永远通过，
// 无法证明它们真的在守护顺序。这里用错误顺序现场搭一个引擎，断言缺失即复现。
func TestMiddlewareOrder_WrongOrderDemonstrablyDropsCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.SlogLogger())
	r.Use(middleware.BodyLimit(1 << 20))
	r.Use(middleware.CORS()) // 错误顺序：应排在 BodyLimit 之前
	r.POST("/api/echo", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/api/echo",
		strings.NewReader(strings.Repeat("a", 2<<20)))
	req.ContentLength = 2 << 20
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码 = %d，期望 413", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("错误顺序下本应缺少 CORS 头，实际得到 %q——说明正序用例没有守住顺序", got)
	}
}
