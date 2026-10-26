package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
)

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
