package llm

import "net/url"

// redactURLString 抹掉 URL 里的 query 取值与 userinfo，只保留协议、主机、路径与参数名。
//
// 中转站地址常常把 key 放在 query（?key=sk-xxx）里；日志要能看出请求打到哪，
// 但不能把凭据一起写进去。参数名保留，值统一换成 ***。
func redactURLString(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return "***"
	}
	safe := *u
	if safe.User != nil {
		safe.User = url.User("***")
	}
	if safe.RawQuery != "" {
		q := safe.Query()
		for k := range q {
			q.Set(k, "***")
		}
		safe.RawQuery = q.Encode()
	}
	safe.Fragment = ""
	safe.RawFragment = ""
	return safe.String()
}
