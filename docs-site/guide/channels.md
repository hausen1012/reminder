# 通知渠道

通道是「消息发到哪里」的配置。一条提醒可以绑定多个通道，触发时会**并发**发送，
每个通道独立记录结果与重试。

## 支持的通道类型

| 类型 | 说明 | 是否需配置 |
|---|---|---|
| `smtp` | 邮件，通过任意 SMTP 服务器发送 | 是 |
| `dingtalk` | 钉钉群机器人 Webhook | 是 |
| `wecom` | 企业微信群机器人 Webhook | 是 |
| `webhook` | 通用 HTTP 回调，可自定义方法、头、Body 模板 | 是 |
| `log` | 输出到服务端日志，用于本地调试 | 否 |

通道类型创建后**不可变更**，因为不同类型的配置结构完全不同。要换类型请新建一个通道。

## SMTP 邮件

| 字段 | 说明 |
|---|---|
| `host` | SMTP 服务器地址，如 `smtp.example.com` |
| `port` | 端口，常用 `465`（SSL）或 `587`（STARTTLS） |
| `username` | 登录用户名 |
| `password_enc` | 密码或授权码，加密存储 |
| `from_addr` | 发件人地址 |
| `from_name` | 发件人显示名，可留空 |
| `to` | 收件人列表，多个用英文逗号分隔 |

连接方式的判定顺序：

1. 显式设置 `security_mode` 时以它为准（`plain` / `starttls` / `implicit_tls`）
2. 否则勾选了 STARTTLS 就用 `starttls`
3. 否则端口是 465 就推断为 `implicit_tls`
4. 其余情况按 `plain`

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

## 企业微信机器人

| 字段 | 说明 |
|---|---|
| `webhook_url` | 群机器人 Webhook 地址（含 `key`） |
| `mentioned_list` | 需要 @ 的成员 userid 列表，`@all` 表示所有人 |
| `mentioned_mobile_list` | 需要 @ 的手机号列表 |

格式处理与钉钉一致：markdown 走 markdown 类型，其余降级为 text。

## 通用 Webhook

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

## 日志通道

不调用任何外部服务，只是把渲染后的内容写进服务端日志。**零配置**，非常适合：

- 本地开发时验证调度是否按预期触发
- 排查占位符渲染结果
- 手上还没有可用的通知渠道时先跑通流程

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
- **测试发送**：用通道自己的配置发一条测试消息，不写日志，用于验证配置正确
- **统计**：通知页可以看到各通道的使用情况

## 发送失败与重试

单个通道发送失败会按固定节奏重试，最多 3 次尝试（立即、10 秒后、30 秒后）。

如果错误是**永久性**的——鉴权失败、参数错误、对端返回业务错误码非 0——则立即停止重试，
避免无意义地反复冲击对端。

具体状态含义见[发送状态与重试](/reference/delivery-status)。
