# 通知渠道

通道是「消息发到哪里」的配置。一条提醒可以绑定多个通道，触发时会**并发**发送，
每个通道独立记录结果与重试。

## 支持的通道类型

| 类型 | 说明 | 是否需配置 |
|---|---|---|
| `smtp` | 邮件，通过任意 SMTP 服务器发送 | 是 |
| `dingtalk` | 钉钉群机器人 Webhook | 是 |
| `wecom` | 企微群机器人 Webhook | 是 |
| `feishu` | 飞书群自定义机器人 Webhook | 是 |
| `webhook` | HTTP 回调，可自定义方法、头、Body 模板 | 是 |
| `telegram` | Telegram Bot，发送到指定会话 | 是 |
| `bark` | Bark iOS 推送，支持自建服务端 | 是 |
| `apprise` | 通过 Apprise API 服务转发到其支持的全部渠道 | 是 |
| `ntfy` | ntfy 话题推送，支持自建服务端与访问令牌 | 是 |

通道类型创建后**不可变更**，因为不同类型的配置结构完全不同。要换类型请新建一个通道。

## 邮件

| 字段 | 说明 |
|---|---|
| `host` | SMTP 服务器地址，如 `smtp.example.com` |
| `port` | 端口，常用 `465`（SSL）或 `587`（STARTTLS） |
| `username` | 登录用户名 |
| `password_enc` | 密码或授权码，加密存储 |
| `from_addr` | 发件人地址 |
| `from_name` | 发件人显示名，可留空 |
| `to` | 收件人列表，多个用英文逗号分隔 |
| `security_mode` | 连接安全模式：`starttls`（默认）、`implicit_tls`（465 端口直连 TLS）、`plain`（明文） |

连接方式由 `security_mode` 决定：

- `starttls`：STARTTLS 加密连接（默认，常用 587 端口）
- `implicit_tls`：直连 TLS/SSL 加密（常用 465 端口）
- `plain`：不加密明文连接（常用 25 端口）

::: tip 用邮箱服务商的「授权码」
国内邮箱（QQ、163 等）通常不能直接用登录密码，需要在邮箱设置里开启 SMTP 服务并生成授权码，
把授权码填到密码栏。
:::

邮件正文的格式处理：

- `text`：`text/plain`
- `markdown`：发送前转换成 HTML
- `html`：原样作为 `text/html` 发送

标题用 UTF-8 + base64 编码，中文不会乱码。

## 钉钉机器人

| 字段 | 说明 |
|---|---|
| `webhook_url` | 群机器人 Webhook 地址（含 `access_token`） |
| `secret_enc` | 加签密钥，机器人安全设置选了「加签」时必填，加密存储 |
| `at_mobiles` | 需要 @ 的手机号列表 |
| `at_all` | 是否 @ 所有人 |

如果机器人安全设置选的是「自定义关键词」，Webhook 地址即可，无需密钥。

格式处理：`markdown` 走钉钉的 markdown 消息类型；`text` 与 `html`（先剥掉标签）走 text 类型。

## 企微机器人

| 字段 | 说明 |
|---|---|
| `webhook_url` | 群机器人 Webhook 地址（含 `key`） |
| `mentioned_list` | 需要 @ 的成员 userid 列表，`@all` 表示所有人 |
| `mentioned_mobile_list` | 需要 @ 的手机号列表 |

格式处理与钉钉一致：markdown 走 markdown 类型，其余降级为 text。

## 飞书机器人

| 字段 | 说明 |
|---|---|
| `webhook_url` | 群机器人 Webhook 地址（`https://open.feishu.cn/open-apis/bot/v2/hook/...`） |
| `secret_enc` | 签名校验密钥，机器人安全设置选了「签名校验」时必填，加密存储 |
| `at_all` | 是否在消息开头 @ 所有人；需要所在群开启「@ 所有人」功能 |

飞书自定义机器人只发送纯文本消息：`markdown` 与 `html` 正文都会先转换为纯文本，
转换规则见[降级为纯文本的结果](/guide/content-format#降级为纯文本的结果)。
正文超过 4000 字时会被截断并追加省略号（对端请求体上限为 20 KB）。

## Webhook

最灵活的通道，可以对接任何接受 HTTP 请求的服务。

| 字段 | 说明 |
|---|---|
| `method` | `GET` 或 `POST`，默认 `POST` |
| `url` | 目标地址 |
| `headers` | 自定义请求头，每行一个 `key: value` |
| `authorization_enc` | `Authorization` 头的内容，加密存储，可留空 |
| `body_template` | POST 时的请求体模板 |
| `query_template` | GET 时的查询参数模板，每行一个 `key: 值模板` |
| `content_type` | 请求的 Content-Type，默认 `application/json` |

模板里可以用全部[内置变量](/reference/variables)，例如：

```json
{
  "title": "{{title}}",
  "content": "{{content}}",
  "at": "{{fire_at}}"
}
```

GET 模式的查询模板示例：

```
text: {{title}} {{content}}
tag: reminder
```

::: warning Webhook 不做格式转换
其他通道会自动把 markdown / html 降级成纯文本，但 Webhook **不会**。
`{{content}}` 拿到的是渲染后的原始内容，格式转换由你的模板自己负责。
:::

## Telegram

| 字段 | 说明 |
|---|---|
| `bot_token_enc` | Bot Token，向 `@BotFather` 申请，加密存储 |
| `chat_id` | 目标用户或群组的 chat_id |
| `disable_notification` | 静默发送，客户端不播放提示音 |

发送前需要把机器人加入目标会话。个人会话的 chat_id 可从 `getUpdates` 接口读取，
群组 chat_id 为负数。

正文超过 4096 字时会被截断并追加省略号。`markdown` 与 `html` 正文都会先转换为纯文本，
转换规则见[降级为纯文本的结果](/guide/content-format#降级为纯文本的结果)。

::: tip 需要代理的网络环境
程序通过 Go 标准库的默认 HTTP 客户端访问 `api.telegram.org`，该客户端会读取
`HTTPS_PROXY` 环境变量。在无法直连的网络里给服务进程设置该变量即可，无需改动通道配置。
:::

## Bark

iOS 推送通道，可以走 Bark 官方服务端，也可以指向自建实例。

| 字段 | 说明 |
|---|---|
| `device_key_enc` | Bark App 首页显示的推送 Key，加密存储 |
| `server_url` | 服务端地址，默认 `https://api.day.app`；自建时填自己的地址 |
| `group` | 通知分组名，可留空 |
| `sound` | 铃声名，可留空 |
| `icon` | 通知图标 URL，iOS 15 及以上生效，可留空 |

正文超过 2000 字时会被截断并追加省略号。`markdown` 与 `html` 正文都会先转换为纯文本，
转换规则见[降级为纯文本的结果](/guide/content-format#降级为纯文本的结果)。

## Apprise

对接已部署的 [Apprise API](https://github.com/caronc/apprise-api) 服务，
借助它把同一条消息转发到自身未内置的渠道。

| 字段 | 说明 |
|---|---|
| `api_url` | Apprise API 服务地址，如 `http://localhost:8000` |
| `key_enc` | Apprise API 里保存的配置 KEY，加密存储 |
| `urls` | 目标列表，每行一个 Apprise URL；填了之后按无状态模式发送 |
| `type` | 消息类型：`info` / `success` / `warning` / `failure` |

两种模式：

- **状态化**：只填 `key_enc`，服务端使用该 KEY 已保存的目标列表发送
- **无状态**：填 `urls`（`key_enc` 只用于鉴权、可留空），目标随请求下发

`key_enc` 与 `urls` 至少要填一个。正文超过 2000 字时会被截断并追加省略号。

服务端返回非 200 时按状态码区分：`424`（没有任何目标发送成功）与 `4xx` 视为永久错误，
`429`、`5xx` 会重试。

## ntfy

| 字段 | 说明 |
|---|---|
| `server_url` | 服务端地址，默认 `https://ntfy.sh`；自建时填自己的地址 |
| `topic` | 话题名，只允许字母、数字、下划线、连字符与点号 |
| `token_enc` | 访问令牌，服务端启用鉴权时填写，加密存储 |
| `priority` | 优先级：`min` / `low` / `default` / `high` / `urgent` |
| `tags` | 标签，多个用英文逗号分隔，客户端会显示成标签或 emoji |

发送时用 `POST {server}/{topic}`，标题、优先级、标签放在请求头，正文即消息体；
配置了访问令牌时以 `Authorization: Bearer <token>` 发送。

正文超过 1200 字时会被截断并追加省略号。

::: warning 公开话题任何人都能订阅
未开启鉴权的 ntfy 服务端上，话题名就是唯一凭据。话题名尽量随机，或在服务端开启访问控制。
:::

## 敏感字段与脱敏

配置中所有以 `_enc` 结尾的字段在落库前用 AES-256-GCM 加密，读取接口返回时统一显示为 `***`。

因此在编辑通道时，**密码框留空表示保持原值不变**——表单会把 `***` 回传给服务端，
服务端看到这个占位符就沿用原密文。只有真的输入了新值才会覆盖。

想彻底清空某个密钥（例如取消钉钉加签），需要在密码框里输入一个空格，界面提示里也写明了这一点。

::: danger 加密密钥变更的影响
`_enc` 字段由 `ENCRYPTION_KEY` 加密。更换该环境变量后旧密文无法解开，
需要到通知页把这些字段重新填写一遍。
:::

## 通道的启用与测试

- **启用 / 禁用**：禁用的通道不参与发送，绑定它的提醒触发时会跳过
- **测试发送**：用通道自己的配置发一条测试消息，不写日志，用于验证配置正确。
  失败时会把具体原因（地址不通、密码错、配置解不开）直接显示出来，而不是笼统的「服务器内部错误」
- **统计**：通知页可以看到各通道的使用情况

## 发送失败与重试

单个通道发送失败会按固定节奏重试，最多 3 次尝试（立即、10 秒后、30 秒后）。

如果错误是**永久性**的——鉴权失败、参数错误、对端返回业务错误码非 0——则立即停止重试，
避免无意义地反复冲击对端。

具体状态含义见[发送状态与重试](/reference/delivery-status)。
