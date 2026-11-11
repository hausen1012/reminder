// spec.go 定义每种通道类型的元数据：显示名、配置字段、能力（格式支持与长度上限）。
//
// 这里是通道类型的唯一权威清单：services 的必填校验、AI 工具的类型枚举、
// /api/channels/types 接口都由它派生。新增通道时在 specs 与 Registry 各登记一次，
// 注册表测试会保证两处键一致。
package notifier

import "strings"

// encSuffix 是敏感配置字段的后缀约定：以它结尾的字段落库前加密、读出时脱敏。
const encSuffix = "_enc"

// 字段类型，前端据此渲染通用表单控件。
const (
	FieldText     = "text"
	FieldPassword = "password" // 敏感字段，键名必须以 _enc 结尾
	FieldNumber   = "number"
	FieldTextarea = "textarea"
	FieldSwitch   = "switch"
	FieldSelect   = "select"
	FieldList     = "list" // 逗号分隔的字符串列表
)

// Option 是 select 字段的可选值。
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// FieldSpec 是单个配置字段的描述。
type FieldSpec struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Kind        string   `json:"kind"`
	Placeholder string   `json:"placeholder,omitempty"`
	Help        string   `json:"help,omitempty"`
	Required    bool     `json:"required"`
	Default     any      `json:"default,omitempty"`
	Options     []Option `json:"options,omitempty"`
}

// Spec 是一种通道类型的完整元数据。
type Spec struct {
	Type        string      `json:"type"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
	Fields      []FieldSpec `json:"fields"`
	// RequireAny 列出「至少填一个」的字段键；为空表示无此约束。
	RequireAny []string `json:"require_any,omitempty"`
	// MaxBodyLen 是正文最大字符数（按 rune 计），0 表示不限。超长时发送前截断。
	MaxBodyLen int `json:"max_body_len"`
}

// specs 是全部通道类型的元数据。
var specs = map[string]Spec{
	"smtp": {
		Type:        "smtp",
		Label:       "邮件 SMTP",
		Description: "通过任意 SMTP 服务器发送邮件",
		Fields: []FieldSpec{
			{Key: "host", Label: "SMTP Host", Kind: FieldText, Placeholder: "smtp.example.com", Required: true},
			{Key: "port", Label: "端口", Kind: FieldNumber, Placeholder: "587", Required: true, Default: 587},
			{Key: "security_mode", Label: "连接方式", Kind: FieldSelect, Default: "starttls", Options: []Option{
				{Value: "starttls", Label: "STARTTLS（常用 587）"},
				{Value: "implicit_tls", Label: "SSL/TLS 直连（常用 465）"},
				{Value: "plain", Label: "明文（常用 25）"},
			}},
			{Key: "username", Label: "用户名", Kind: FieldText, Placeholder: "登录用户名"},
			{Key: "password_enc", Label: "密码 / 授权码", Kind: FieldPassword, Placeholder: "密码或授权码"},
			{Key: "from_addr", Label: "发件人地址", Kind: FieldText, Placeholder: "sender@example.com", Required: true},
			{Key: "from_name", Label: "发件人显示名", Kind: FieldText, Placeholder: "提醒助手"},
			{Key: "to", Label: "收件人", Kind: FieldList, Placeholder: "多个地址用英文逗号分隔", Required: true},
		},
	},
	"dingtalk": {
		Type:        "dingtalk",
		Label:       "钉钉机器人",
		Description: "钉钉群自定义机器人 Webhook",
		Fields: []FieldSpec{
			{Key: "webhook_url", Label: "Webhook URL", Kind: FieldText, Placeholder: "https://oapi.dingtalk.com/robot/send?access_token=...", Required: true},
			{Key: "secret_enc", Label: "加签密钥", Kind: FieldPassword, Help: "机器人安全设置选择「加签」时必填"},
			{Key: "at_mobiles", Label: "@ 手机号", Kind: FieldList, Placeholder: "多个手机号用英文逗号分隔"},
			{Key: "at_all", Label: "@ 所有人", Kind: FieldSwitch},
		},
	},
	"wecom": {
		Type:        "wecom",
		Label:       "企业微信机器人",
		Description: "企业微信群自定义机器人 Webhook",
		Fields: []FieldSpec{
			{Key: "webhook_url", Label: "Webhook URL", Kind: FieldText, Placeholder: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...", Required: true},
			{Key: "mentioned_list", Label: "@ 成员 userid", Kind: FieldList, Placeholder: "@all 表示所有人，多个用英文逗号分隔"},
			{Key: "mentioned_mobile_list", Label: "@ 成员手机号", Kind: FieldList, Placeholder: "多个手机号用英文逗号分隔"},
		},
	},
	"feishu": {
		Type:        "feishu",
		Label:       "飞书机器人",
		Description: "飞书群自定义机器人 Webhook",
		Fields: []FieldSpec{
			{Key: "webhook_url", Label: "Webhook URL", Kind: FieldText, Placeholder: "https://open.feishu.cn/open-apis/bot/v2/hook/...", Required: true},
			{Key: "secret_enc", Label: "签名校验密钥", Kind: FieldPassword, Help: "机器人安全设置选择「签名校验」时必填"},
			{Key: "at_all", Label: "@ 所有人", Kind: FieldSwitch, Help: "需要群开启 @ 所有人 功能"},
		},
		// 飞书文本消息请求体上限 20 KB，这里留足余量
		MaxBodyLen: 4000,
	},
	"telegram": {
		Type:        "telegram",
		Label:       "Telegram",
		Description: "通过 Bot API 发送到指定会话；需要网络能访问 api.telegram.org，可用 HTTPS_PROXY 配置代理",
		Fields: []FieldSpec{
			{Key: "bot_token_enc", Label: "Bot Token", Kind: FieldPassword, Placeholder: "123456:ABC-DEF...", Required: true, Help: "向 @BotFather 申请"},
			{Key: "chat_id", Label: "Chat ID", Kind: FieldText, Placeholder: "用户或群组的 chat_id", Required: true, Help: "把机器人拉进目标会话后从 getUpdates 获取"},
			{Key: "disable_notification", Label: "静默发送", Kind: FieldSwitch, Help: "客户端不播放提示音"},
		},
		MaxBodyLen: 4096,
	},
	"bark": {
		Type:        "bark",
		Label:       "Bark",
		Description: "iOS 推送，支持官方与自建服务端",
		Fields: []FieldSpec{
			{Key: "device_key_enc", Label: "Device Key", Kind: FieldPassword, Required: true, Help: "Bark App 首页显示的推送 Key"},
			{Key: "server_url", Label: "服务端地址", Kind: FieldText, Placeholder: "https://api.day.app", Default: DefaultBarkServer, Help: "自建服务端填自己的地址"},
			{Key: "group", Label: "分组", Kind: FieldText, Placeholder: "通知分组名"},
			{Key: "sound", Label: "铃声", Kind: FieldText, Placeholder: "如 alarm"},
			{Key: "icon", Label: "图标 URL", Kind: FieldText, Placeholder: "https://example.com/icon.png"},
		},
		MaxBodyLen: 2000,
	},
	"ntfy": {
		Type:        "ntfy",
		Label:       "ntfy",
		Description: "发布到 ntfy 话题，支持自建服务端",
		Fields: []FieldSpec{
			{Key: "server_url", Label: "服务端地址", Kind: FieldText, Placeholder: "https://ntfy.sh", Default: DefaultNtfyServer, Help: "自建服务端填自己的地址"},
			{Key: "topic", Label: "话题", Kind: FieldText, Placeholder: "topic 名，仅字母数字与 _ -", Required: true, Help: "公开话题任何人只要知道名字就能订阅"},
			{Key: "token_enc", Label: "访问令牌", Kind: FieldPassword, Help: "服务端启用鉴权时填写，以 Bearer 形式发送"},
			{Key: "priority", Label: "优先级", Kind: FieldSelect, Default: "default", Options: []Option{
				{Value: "min", Label: "最低"},
				{Value: "low", Label: "低"},
				{Value: "default", Label: "默认"},
				{Value: "high", Label: "高"},
				{Value: "urgent", Label: "紧急"},
			}},
			{Key: "tags", Label: "标签", Kind: FieldText, Placeholder: "多个标签用英文逗号分隔，如 warning,robot"},
		},
		MaxBodyLen: 1200,
	},
	"apprise": {
		Type:        "apprise",
		Label:       "Apprise",
		Description: "通过 Apprise API 服务转发到任意受支持的渠道",
		RequireAny:  []string{"key_enc", "urls"},
		Fields: []FieldSpec{
			{Key: "api_url", Label: "API 地址", Kind: FieldText, Placeholder: "http://localhost:8000", Required: true, Default: "http://localhost:8000"},
			{Key: "key_enc", Label: "配置 KEY", Kind: FieldPassword, Help: "Apprise API 里保存的配置 KEY；留空时必须在 URLs 里填写目标"},
			{Key: "urls", Label: "URLs", Kind: FieldTextarea, Placeholder: "每行一个 Apprise URL，填写后按无状态模式发送", Help: "示例：discord://webhook_id/webhook_token"},
			{Key: "type", Label: "消息类型", Kind: FieldSelect, Default: "info", Options: []Option{
				{Value: "info", Label: "info"},
				{Value: "success", Label: "success"},
				{Value: "warning", Label: "warning"},
				{Value: "failure", Label: "failure"},
			}},
		},
		MaxBodyLen: 2000,
	},
	"webhook": {
		Type:        "webhook",
		Label:       "Webhook",
		Description: "可自定义方法、请求头与 Body 模板的 HTTP 回调",
		Fields: []FieldSpec{
			{Key: "method", Label: "方法", Kind: FieldSelect, Default: "POST", Options: []Option{
				{Value: "POST", Label: "POST"},
				{Value: "GET", Label: "GET"},
			}},
			{Key: "url", Label: "URL", Kind: FieldText, Required: true},
			{Key: "headers", Label: "请求头", Kind: FieldTextarea, Help: "每行一个 key: value"},
			{Key: "authorization_enc", Label: "Authorization", Kind: FieldPassword},
			{Key: "body_template", Label: "Body 模板", Kind: FieldTextarea, Help: "可用 {{title}} {{content}} 等占位符"},
			{Key: "query_template", Label: "查询参数模板", Kind: FieldTextarea, Help: "GET 时使用，每行一个 key: 值模板"},
			{Key: "content_type", Label: "Content-Type", Kind: FieldText, Default: "application/json"},
		},
	},
}

// specOrder 决定类型枚举与前端下拉框的顺序。
var specOrder = []string{"smtp", "dingtalk", "wecom", "feishu", "telegram", "bark", "ntfy", "apprise", "webhook"}

// SpecOf 按类型取元数据。
func SpecOf(typ string) (Spec, bool) {
	s, ok := specs[typ]
	return s, ok
}

// Specs 按 specOrder 返回全部元数据。
func Specs() []Spec {
	out := make([]Spec, 0, len(specOrder))
	for _, typ := range specOrder {
		out = append(out, specs[typ])
	}
	return out
}

// Types 按 specOrder 返回全部类型名。
func Types() []string {
	out := make([]string, len(specOrder))
	copy(out, specOrder)
	return out
}

// IsSensitiveKey 判断配置键是否为敏感字段（约定以 _enc 结尾）。
func IsSensitiveKey(key string) bool {
	return strings.HasSuffix(key, encSuffix)
}
