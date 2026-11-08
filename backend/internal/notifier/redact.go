package notifier

import (
	"net/http"
	"net/url"
	"sort"
)

// 写入日志前的脱敏工具：钉钉/企微的 webhook URL 自带 access_token，Webhook 通道还可能有
// Authorization 头，凡是要记 URL 或 header 的地方一律走这里。
//
// redactURL 抹掉 query 取值与 userinfo，只保留协议、主机、路径与参数名。例如
// https://oapi.dingtalk.com/robot/send?access_token=abc&sign=def
// → https://oapi.dingtalk.com/robot/send?access_token=***&sign=***
func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	safe := *u
	if safe.User != nil {
		// 带上 userinfo 的 URL 极少见，一旦有必然是凭据
		safe.User = url.User("***")
	}
	if safe.RawQuery != "" {
		q := safe.Query()
		for k := range q {
			q.Set(k, "***")
		}
		safe.RawQuery = q.Encode()
	}
	// Fragment 可能被用来塞 token，统一丢弃
	safe.Fragment = ""
	safe.RawFragment = ""
	return safe.String()
}

// redactURLString 是 redactURL 的字符串入参版本；解析失败时只返回主机部分。
func redactURLString(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "***"
	}
	return redactURL(u)
}

// headerKeys 返回 header 名列表，只记名字不记值（Authorization 等本身就是凭据）。
func headerKeys(h http.Header) []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
