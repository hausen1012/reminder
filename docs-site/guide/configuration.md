# 配置项

系统配置分两类：**启动时通过环境变量注入的配置**，和**在面板里随时可改的站点配置**。

## 环境变量

环境变量只在进程启动时读取，修改后需要重启服务。仓库根目录的 `.env.example` 列出了可直接复制使用的模板。

### 基础

| 变量 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `8765` | 监听端口 |
| `DB_PATH` | `/data/db/reminder.db` | SQLite 数据库文件路径，目录不存在会自动创建 |
| `USERNAME` | `admin` | 初始管理员用户名 |
| `PASSWORD` | `admin123` | 初始管理员密码 |

::: warning 初始账号只在空库时使用
`USERNAME` / `PASSWORD` 仅在 `users` 表为空时用于创建第一个账号。库中已有账号后改这两个变量
不会有任何效果，请到面板的**设置**页修改密码。
:::

### 安全

| 变量 | 默认值 | 说明 |
|---|---|---|
| `JWT_SECRET` | 随机生成 | 登录令牌（JWT）的签名密钥。留空时每次启动随机生成，**重启后旧令牌全部失效**，需要重新登录 |
| `ENCRYPTION_KEY` | 内置 fallback | 通道敏感字段（SMTP 密码、机器人加签密钥、Bot Token、Webhook Authorization 等）与**令牌明文**的加密密钥 |

`ENCRYPTION_KEY` 要求是 **base64 编码的 32 字节**（AES-256）。生成方式：

```bash
# 任意平台（Git Bash 里也能用）
openssl rand -base64 32
```

```powershell
# PowerShell 原生方式
$b = New-Object byte[] 32
(New-Object System.Security.Cryptography.RNGCryptoServiceProvider).GetBytes($b)
[Convert]::ToBase64String($b)
```

未配置时会退回到编译进二进制的固定密钥，安全等级等同明文——**数据库文件泄漏即等于密码泄漏**。
生产环境务必显式配置；启动日志里出现 `ENCRYPTION_KEY 未配置` 的 `WARN` 时就该补上。

::: danger 换密钥会导致旧数据读不出来
通道配置与令牌明文都以密文入库。更换 `ENCRYPTION_KEY` 后两者都无法解密：前者到**通知**页
重新填一遍，后者点「查看」会提示重新创建（令牌本身仍能调用，校验用的是独立保存的哈希）。
:::

### 链接与时间

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SITE_URL` | `http://localhost:8765` | 站点对外地址的**初始值**；用于拼接确认链接 `/c/:token` 与日志里的确认地址。之后以设置页里的「站点地址」为准 |
| `TIMEZONE` | `Asia/Shanghai` | **首次启动**时的站点时区默认值；之后以设置页里的时区为准 |

::: tip 站点地址与时区都在设置页改，不用改环境变量
站点时区（`config` 表 `key=timezone`）可在面板**设置 → 时区**随时修改，保存后立即重建调度器、
按新时区重算所有启用提醒的下次触发时间并写回数据库。`TIMEZONE` 只在库里还没有该配置时作为初始值。

站点地址（`config` 表 `key=site_url`）同理：面板**设置 → 站点地址**改完立即生效，
`SITE_URL` 只在库里还没有该配置时作为初始值。
:::

改时区会改变**下次触发的绝对时刻**：调度规则存的是本地时间（如 `09:00` 即「站点当地 9 点」），
时区一变对应的 UTC 时刻就跟着变。重算以当前时间为基准，因此不会补发一串错过的提醒；
单次提醒若在新时区下已过期则下次触发时间清空，但不会自动禁用。时区未变化时不触发重建。

`SITE_URL`（或设置页里的「站点地址」）必须填**用户手机上能访问到的地址**：
填 `localhost` 的话手机点开是空的。地址必须以 `http://` 或 `https://` 开头，结尾斜杠会被去掉。

### 调度与日志

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SWEEP_INTERVAL_SEC` | `60` | 兜底扫描间隔（秒）。扫描用于补发进程重启期间错过的触发 |
| `MISS_TOLERANCE_MINUTES` | `60` | 漏触发容忍窗口（分钟）。早于该窗口的漏触发直接标记为 `expired`，不再补发 |
| `LOG_AUTO_PURGE_DAYS` | `0` | 日志自动清理保留天数；`0` 表示不自动清理。大于 0 时每天凌晨 3 点清理一次 |
| `LOG_FILE` | `/var/log/reminder` | 日志目录。日志总是同时写标准错误，目录创建失败时只剩标准错误 |
| `LOG_FORMAT` | `text` | 日志输出格式。`text` 是 `[INFO] 时间 \| request_id \| 消息 k=v` 的一行制式，便于人工排查；`json` 便于接入日志采集（Loki / ELK 等） |
| `LOG_LEVEL` | `info` | 最低日志级别：`debug` / `info` / `warn` / `error`。默认级别下**成功请求的访问日志不输出**（降到 `debug`）；`debug` 会额外打印每条 SQL、访问日志与通道发送细节 |

### 请求与访问控制

| 变量 | 默认值 | 说明 |
|---|---|---|
| `MAX_BODY_MB` | `1` | 单个请求体上限（MB），超出返回 `413 payload_too_large` |
| `TRUSTED_PROXIES` | 空 | 可信反向代理的 IP / CIDR，逗号分隔，如 `127.0.0.1,172.18.0.0/16` |
| `LOGIN_MAX_ATTEMPTS` | `5` | 同一「用户名 + IP」连续登录失败多少次后临时锁定；`0` 表示关闭登录限流 |
| `LOGIN_LOCKOUT_MINUTES` | `15` | 登录锁定时长（分钟） |
| `HTTPS_PROXY` | 空 | 访问外部服务时的 HTTP 代理，作用于 Telegram 等需要代理的网络环境 |

::: tip 放在反向代理后面时务必配置 TRUSTED_PROXIES
访问日志里的 `ip` 字段、登录限流的计数键都依赖客户端 IP。
默认情况下服务端**不信任任何代理头**，看到的是 TCP 对端地址——反向代理后面
那就是代理自己的 IP，所有请求会被算成同一个来源。

配置成你实际使用的代理地址（Nginx 的 `127.0.0.1`、Docker 网段等）之后，
才会从 `X-Forwarded-For` 里取真实客户端 IP。**不要**填 `0.0.0.0/0`：
那等于重新允许任何客户端伪造自己的 IP。
:::

::: warning 登录限流会锁定你的账号
连续输错密码达到 `LOGIN_MAX_ATTEMPTS` 次后，该「用户名 + IP」组合在
`LOGIN_LOCKOUT_MINUTES` 内直接返回 `429`，即使密码正确也进不去。
换一个网络（即换 IP）可以立刻再试；等锁定时间过去也会自动解锁。
:::

### 日志级别

`LOG_LEVEL` 决定输出到哪一级，默认 `info`；各级的典型内容与排查方式见[日志与监控](/guide/logs-and-monitoring)。

`MISS_TOLERANCE_MINUTES` 需要跟你的使用习惯匹配：希望宕机半天恢复后仍收到提醒就调大，
希望过期的就别再吵就保持默认。

## 站点配置

面板**设置**页里可以修改的配置存在数据库中，改完立即生效，不需要重启：

| 键 | 说明 |
|---|---|
| `app_name` | 站点名称，显示在侧边栏（移动端为顶栏）与浏览器标题 |
| `logo_svg` | 自定义 Logo 的 SVG 源码 |
| `timezone` | 站点时区（如 `Asia/Shanghai`）。保存后立即生效，所有启用提醒按新时区重算下次触发时间并写回数据库。值未变化时不触发重建 |
| `site_url` | 站点对外地址，用于拼接确认链接。改完立即生效，为空时回退到 `SITE_URL` |

值为空字符串时表示删除该配置项，回退到内置默认（`timezone` 回退到 `Asia/Shanghai`）。

`/api/config` 是**白名单**接口：只接受上表中的键，写入其它键（包括 `ai_` 前缀）
一律返回 `400 validation_failed`，读取时也只返回这些键。详见[内置 API](/reference/internal-api#api-config-的白名单)。

AI 助手的配置也以 `ai_` 前缀存在同一张表里，但**不通过 `/api/config` 读写**，
而是走 `GET/PUT /api/ai/settings`：

| 键 | 说明 |
|---|---|
| `ai_enabled` | 是否启用 AI 助手 |
| `ai_provider` | `anthropic` / `openai`（OpenAI 兼容协议） |
| `ai_base_url` | 厂商地址；Anthropic 留空走官方 |
| `ai_model` | 模型名 |
| `ai_api_key_enc` | 加密后的 API Key，读出时恒显示 `***` |
| `ai_custom_prompt` | 用户自定义的追加提示词，拼接在内置系统提示词之后 |
| `ai_max_retries` | 接口失败后的最大重试次数，默认 `10`，范围 0–100（0 表示不重试） |

密钥单独走 AI 设置接口，避免通用配置接口把密钥带出面板以外的地方。
详见[AI 助手](/guide/ai-assistant)。

::: tip 没有新增环境变量
AI 助手的全部配置都在面板里改，每次对话都按最新设置构造客户端，不需要重启进程。
对话内容不写入数据库，`config` 表里的 `ai_*` 只保存厂商、模型与加密后的密钥。
:::

## Docker 部署示例

```yaml
services:
  reminder:
    build: .
    ports:
      - "8765:8765"
    volumes:
      - ./reminder/db:/data/db
      - ./reminder/log:/var/log/reminder
    environment:
      - USERNAME=admin
      - PASSWORD=换一个强密码
      - JWT_SECRET=换一个随机串
      - ENCRYPTION_KEY=base64编码的32字节
      - SITE_URL=https://reminder.example.com
      - TIMEZONE=Asia/Shanghai
```
