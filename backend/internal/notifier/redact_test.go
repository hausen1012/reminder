package notifier

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// URL 里的 query 取值一律抹掉，只保留参数名。
// 钉钉 / 企微的凭据就在 query 里（access_token、key、sign），不能落日志。
func TestRedactURLString(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "钉钉 access_token 与 sign",
			in:   "https://oapi.dingtalk.com/robot/send?access_token=SECRET&sign=SIG",
			want: "https://oapi.dingtalk.com/robot/send?access_token=%2A%2A%2A&sign=%2A%2A%2A",
		},
		{
			name: "企微 key",
			in:   "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc-123",
			want: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=%2A%2A%2A",
		},
		{
			name: "无 query 时原样返回",
			in:   "https://example.com/hook",
			want: "https://example.com/hook",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactURLString(tc.in)
			if got != tc.want {
				t.Fatalf("redactURLString(%q) = %q，期望 %q", tc.in, got, tc.want)
			}
			// 关键断言：原文里的凭据值不能出现在输出里
			if u, err := url.Parse(tc.in); err == nil {
				for _, vals := range u.Query() {
					for _, v := range vals {
						if len(v) >= 4 && strings.Contains(got, v) {
							t.Fatalf("输出里仍包含凭据值 %q: %q", v, got)
						}
					}
				}
			}
		})
	}
}

// userinfo 也是凭据，必须抹掉。
func TestRedactURL_UserInfo(t *testing.T) {
	u, err := url.Parse("https://user:pass@example.com/hook")
	if err != nil {
		t.Fatal(err)
	}
	got := redactURL(u)
	if strings.Contains(got, "pass") {
		t.Fatalf("输出里仍包含密码: %q", got)
	}
}

// headerKeys 只返回名字，不返回值。
func TestHeaderKeys(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer SECRET")
	h.Set("Content-Type", "application/json")

	keys := headerKeys(h)
	if len(keys) != 2 {
		t.Fatalf("keys = %v，期望 2 个", keys)
	}
	if keys[0] != "Authorization" || keys[1] != "Content-Type" {
		t.Fatalf("keys = %v，期望按字母序排列", keys)
	}
	for _, k := range keys {
		if strings.Contains(k, "SECRET") {
			t.Fatalf("key 里混入了 header 取值: %q", k)
		}
	}
}
