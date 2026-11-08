# AI 助手接口

面板右下角 AI 浮窗使用的接口，鉴权方式同为 `Authorization: Bearer <jwt>`。

这些接口**只在登录后的面板可用**，外部令牌（`X-AUTH`）无法调用 AI 相关路径。

## 设置

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/ai/settings` | 读取 AI 设置，密钥回显为 `***` |
| `PUT` | `/api/ai/settings` | 保存设置 |
| `POST` | `/api/ai/test` | 用当前/待保存的配置发一次最小请求 |
| `POST` | `/api/ai/models` | 按当前/待保存的配置拉取厂商的可用模型列表 |

`PUT /api/ai/settings` 请求体：

```json
{
  "enabled": true,
  "provider": "openai",
  "base_url": "https://api.deepseek.com",
  "model": "deepseek-chat",
  "api_key": "sk-xxx",
  "custom_prompt": "称呼我为老板，回复尽量一句话说完",
  "max_retries": 10
}
```

| 字段 | 说明 |
|---|---|
| `provider` | `anthropic` 或 `openai`（后者指 OpenAI 兼容协议） |
| `base_url` | 留空时：`anthropic` 走官方地址，`openai` 走 `https://api.openai.com` |
| `model` | 模型名；留空时按厂商给默认值 |
| `api_key` | 传 `"***"` 表示保留已保存的密钥；传空串表示清除；传新值则覆盖 |
| `custom_prompt` | 追加在内置系统提示词之后的用户自定义内容；空串表示不用。最多 4000 字，超出返回 `validation_failed` |
| `max_retries` | 接口失败后的最大重试次数。**省略**表示用默认值 `10`；显式传 `0` 表示不重试。合法范围 0–100（负数越界），越界返回 `validation_failed` |

`custom_prompt` 会跟在内置规则之后、并注明「冲突时以内置规则为准」，
因此不能用来绕过待确认机制。`GET /api/ai/settings` 会原样返回该字段（它不是密钥，不做脱敏）。

`enabled=true` 且库里没有已保存密钥、请求也没带新密钥时，返回 `validation_failed`。

`POST /api/ai/test` 请求体同上，返回：

```json
{ "code": 200, "message": "ok", "data": { "reply": "OK" } }
```

连接失败返回 `validation_failed`，`message` 里带上对端的原始错误，便于判断是密钥错、地址错还是模型名错。

`POST /api/ai/models` 的请求体与 `PUT /api/ai/settings` 相同（同样以 `"***"` 表示沿用已保存的密钥），返回：

```json
{ "code": 200, "message": "ok", "data": { "items": ["deepseek-chat", "deepseek-reasoner"] } }
```

设置页的「获取模型列表」按钮用它填充下拉候选。拉取失败（地址错、密钥错、厂商不提供该接口）
返回 `validation_failed`，此时仍可手动填写模型名。

## 对话

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/ai/chat` | 发送一条消息，返回回复文本 + 本轮生成的待确认操作 |
| `POST` | `/api/ai/chat/stream` | 同上，但用 SSE 推送「思考中 / 正在重试 / 完成」状态 |

`POST /api/ai/chat` 请求体：

```json
{
  "message": "每天早上 9 点提醒我喝水，发到钉钉",
  "history": [
    { "role": "user", "content": "现在有哪些通道？" },
    { "role": "assistant", "content": "你有一个钉钉机器人。" }
  ],
  "model": "deepseek-reasoner"
}
```

| 字段 | 说明 |
|---|---|
| `message` | 本轮用户输入，不能为空，最多 4000 字 |
| `history` | 此前对话（不含本轮），最多生效 30 条；由浏览器持有并上传，服务端不保存 |
| `model` | 可选。**仅本次请求**使用的模型，覆盖设置里保存的 `model`，不会写回设置。留空则用设置值 |

响应：

```json
{
  "code": 200,
  "message": "ok",
  "data": {
    "reply": "已生成待确认卡片，点击确认后生效",
    "retries": 0,
    "actions": [
      {
        "id": "6f0c...",
        "kind": "create_reminder",
        "summary": "创建提醒：喝水",
        "status": "pending",
        "created_at": "...",
        "updated_at": "..."
      }
    ]
  }
}
```

::: warning 接口无状态，对话不入库
`history` 由浏览器持有并随每次请求上传，服务端不保存聊天记录，也不返回 `conversation_id`。
最多生效 30 条，非法角色与空内容会被忽略。

`actions` 只包含**本轮**新生成的草稿；历史草稿走下面的列表接口。
`retries` 是本次对话因临时性失败重试的总次数，正常为 `0`。
:::

### 状态流（SSE）

请求体与 `/api/ai/chat` 完全相同，响应为 `text/event-stream`；只推过程状态，不逐字输出回复。

事件格式（每个 `data` 都是一行 JSON）：

```
event: status
data: {"phase":"thinking","text":"助手正在思考…"}

event: retry
data: {"attempt":1,"max":10,"delay_ms":200,"reason":"服务繁忙（限流）"}

event: done
data: {"reply":"…","actions":[…],"retries":1}

event: error
data: {"code":"validation_failed","message":"AI 调用失败: …"}
```

| 事件 | 何时推送 |
|---|---|
| `status` | 每轮开始思考前、工具执行前；`phase` 为 `thinking` / `tools` |
| `retry` | 每次重试**即将开始等待之前**；`delay_ms` 是接下来会等多久，`reason` 是失败原因的简述 |
| `done` | 正常结束，`data` 与 `/api/ai/chat` 的 `data` 结构一致 |
| `error` | 已经开推流之后出错；结构同统一错误体里的 `error` 对象 |

::: warning 开推流后不再走 JSON 错误体
HTTP 状态恒为 `200`，错误通过 `event: error` 下发。只有在写第一个字节之前就失败
（例如请求体不是合法 JSON）时才返回常规的 `{"error":{...}}` 与 4xx。
:::

## 待确认操作

| 方法 | 路径 | 说明 |
|---|---|---|
| `GET` | `/api/ai/actions` | 列出草稿；`?status=pending` 只取待确认，默认返回全部（最多 50 条，按时间倒序） |
| `POST` | `/api/ai/actions/:id/apply` | 执行一条待确认操作 |
| `POST` | `/api/ai/actions/:id/reject` | 拒绝一条待确认操作 |

`GET /api/ai/actions` 响应：

```json
{ "code": 200, "message": "ok", "data": { "items": [ /* AIAction */ ] } }
```

`kind` 取值与执行效果：

| `kind` | 执行动作 |
|---|---|
| `create_channel` | 调用通道服务新建通道（名称冲突等由通道服务报错） |
| `create_reminder` | 新建提醒，来源记为 `ai`；草稿入参中的 `require_confirm` / `confirm_retry_interval_sec` / `confirm_max_retries` 一并落库 |
| `update_reminder` | 用草稿里的完整入参覆盖提醒（含确认配置） |
| `delete_reminder` | 永久删除该提醒 |

确认配置由模型从用户话里提取：只要求「需要确认」而未给数值，或缺省值低于下限
（间隔 < 60 秒、次数 < 1）时，工具层按 300 秒 / 3 次补齐，草稿摘要会标注该策略。
`list_reminders` 的结果包含这三个字段，便于模型判断提醒当前的确认状态。

错误约定：

| 状态码 | 场景 |
|---|---|
| `404` | 操作 id 不存在 |
| `409` | 操作已被处理过（已执行 / 已拒绝 / 失败），不能重复执行 |
| `400` | 执行时校验失败（如名称重复、通道已被删除） |

执行失败时操作状态会变成 `failed`，`error` 字段记录原因。

## 数据表

只有一张表：

| 表 | 说明 |
|---|---|
| `ai_actions` | 待确认操作与执行结果（`payload_enc` 为加密后的入参 JSON） |

对话（用户说了什么、助手回了什么）**不落库**，只存在于浏览器 `sessionStorage`。

`ai_actions.payload_enc` 与通道敏感字段使用同一个 `ENCRYPTION_KEY`；
更换密钥后旧草稿无法解密，执行时会提示重新生成。

### 自动清理

`ai_actions` 里的记录**保留 90 天**（按 `created_at` 计算），每天凌晨 3 点清理一次，
包含仍处于 `pending` 的草稿。没有对应的环境变量或面板开关。

## 涉及的配置键

AI 设置存在 `config` 表，前缀 `ai_`：`ai_enabled`、`ai_provider`、`ai_base_url`、
`ai_model`、`ai_api_key_enc`、`ai_custom_prompt`、`ai_max_retries`。
它们不通过 `/api/config` 暴露。

`ai_custom_prompt` 为明文（不是密钥），值为空时该行被删除，回退到只用内置提示词。
`ai_max_retries` 缺失或非法时回退默认值 `10`；存为 `0` 表示明确关闭重试。

单次回复的输出上限固定为 `4096` 由后端下发，设置接口不识别该字段（传了会被忽略）。
