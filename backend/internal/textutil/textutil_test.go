package textutil

import (
	"net/url"
	"testing"
)

func TestRedactURLString(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"无 query", "https://example.com/path", "https://example.com/path"},
		{"query 取值脱敏", "https://example.com/path?key=secret", "https://example.com/path?key=%2A%2A%2A"},
		{"多参数全部脱敏", "https://example.com/p?access_token=a&sign=b", "https://example.com/p?access_token=%2A%2A%2A&sign=%2A%2A%2A"},
		{"userinfo 脱敏", "https://user:pass@example.com/p", "https://%2A%2A%2A@example.com/p"},
		{"fragment 丢弃", "https://example.com/p#token=abc", "https://example.com/p"},
		{"非法 URL 返回占位", "://bad", "***"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RedactURLString(tc.in); got != tc.want {
				t.Fatalf("RedactURLString(%q) = %q，期望 %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRedactURL_NilAndUserInfo(t *testing.T) {
	if got := RedactURL(nil); got != "" {
		t.Fatalf("nil URL = %q，期望空串", got)
	}

	u, err := url.Parse("https://bob:secret@example.com/x?t=1")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	got := RedactURL(u)
	if u.User.Username() != "bob" {
		t.Fatal("RedactURL 不应修改入参 URL")
	}
	if got == "" || got == u.String() {
		t.Fatalf("未脱敏: %q", got)
	}
}

func TestRedactQuery(t *testing.T) {
	if got := RedactQuery(""); got != "" {
		t.Fatalf("空 query = %q，期望空串", got)
	}
	if got := RedactQuery("a=1&b=2"); got != "a=%2A%2A%2A&b=%2A%2A%2A" {
		t.Fatalf("RedactQuery = %q", got)
	}
	if got := RedactQuery("%zz=1"); got != "***" {
		t.Fatalf("非法 query = %q，期望 ***", got)
	}
}

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"未超长原样返回", "你好", 5, "你好"},
		{"恰好等于上限", "你好", 2, "你好"},
		{"超长追加省略号", "你好世界", 2, "你好…"},
		{"按字符而非字节", "你好世界", 3, "你好世…"},
		{"上限为 0 返回空串", "abc", 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TruncateRunes(tc.in, tc.n); got != tc.want {
				t.Fatalf("TruncateRunes(%q, %d) = %q，期望 %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}
