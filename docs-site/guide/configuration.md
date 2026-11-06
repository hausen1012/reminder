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
| `ENCRYPTION_KEY` | 内置 fallback | 通道敏感字段（SMTP 密码、钉钉加签密钥、Webhook Authorization）与**令牌明文**的加密密钥 |

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

未配置时会退回到编译进二进制的固定密钥，安全等级等同于明文——**数据库文件泄漏即等于密码泄漏**。
生产环境务必显式配置。启动日志里会出现一条 `ENCRYPTION_KEY 未配置` 的 `WARN`，看到它就该补上。

::: danger 换密钥会导致旧数据读不出来
通道配置与令牌明文都以密文入库。更换 `ENCRYPTION_KEY` 后，已有通道的敏感字段与
令牌明文都将无法解密：前者需要到**通知**页把对应字段重新填一遍，后者点「查看」会提示
重新创建（令牌本身仍能用于调用，校验用的是独立保存的哈希）。
两种情况下面板都不报服务器错误。
:::

### 链接与时间

| 变量 | 默认值 | 说明 |
|---|---|---|
| `BASE_URL` | `http://localhost:8765` | 对外可访问的根地址，用于拼接确认链接 `/c/:token` 与日志里的确认地址 |
| `TIMEZONE` | `Asia/Shanghai` | **首次启动**时的站点时区默认值；之后以设置页里的时区为准 |

::: tip 时区在设置页改，不用改环境变量
站点时区存在数据库里（`config` 表 `key=timezone`），面板**设置 → 时区**可随时修改，
保存后立即重建调度器、按新时区重算所有启用提醒的下次触发时间并同步写回数据库，
不需要重启进程。`TIMEZONE` 只在库里还没有该配置时作为初始值使用。
:::

改时区会改变**下次触发的绝对时刻**，这是有意的：调度规则里存的是本地时间（如 `09:00`），
含义是「站点当地 9 点」，所以时区一变，对应的 UTC 时刻跟着变。重算以**当前时间**为基准，
因此不会因为改了时区就补发一串错过的提醒；单次提醒若在新时区下已经过期，
下次触发时间会被清空（不会自动禁用，开关仍由你决定）。保存时若时区没有实际变化，
不会触发重建。

`BASE_URL` 必须填**用户手机上能访问到的地址**。如果只在局域网里自用，填内网 IP；
否则确认链接会指向 `localhost`，手机上点开是空的。

### 调度与日志

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SWEEP_INTERVAL_SEC` | `60` | 兜底扫描间隔（秒）。扫描用于补发进程重启期间错过的触发 |
| `MISS_TOLERANCE_MINUTES` | `60` | 漏触发容忍窗口（分钟）。早于该窗口的漏触发直接标记为 `expired`，不再补发 |
| `LOG_AUTO_PURGE_DAYS` | `0` | 日志自动清理保留天数；`0` 表示不自动清理。大于 0 时每天凌晨 3 点清理一次 |
| `LOG_FILE` | `/var/log/reminder` | 日志目录。日志总是同时写标准错误，目录创建失败时只剩标准错误 |
| `LOG_FORMAT` | `text` | 日志输出格式。`text` 便于人工排查，`json` 便于接入日志采集（Loki / ELK 等） |
| `LOG_LEVEL` | `info` | 最低日志级别：`debug` / `info` / `warn` / `error`。`debug` 会额外打印每条 SQL 与通道发送的细节步骤 |

### 请求与访问控制

| 变量 | 默认值 | 说明 |
|---|---|---|
| `MAX_BODY_MB` | `1` | 单个请求体上限（MB），超出返回 `413 payload_too_large` |
| `TRUSTED_PROXIES` | 空 | 可信反向代理的 IP / CIDR，逗号分隔，如 `127.0.0.1,172.18.0.0/16` |
| `LOGIN_MAX_ATTEMPTS` | `5` | 同一「用户名 + IP」连续登录失败多少次后临时锁定；`0` 表示关闭登录限流 |
| `LOGIN_LOCKOUT_MINUTES` | `15` | 登录锁定时长（分钟） |

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

### 日志里有什么

服务端日志可以按级别筛，排查时先看 `warn` 与 `error`：

| 级别 | 什么时候出现 |
|---|---|
| `error` | 服务端故障：SQL 失败、panic、内部错误（响应 5xx）、提醒注册调度器失败、投递最终失败 |
| `warn` | 客户端问题或可恢复异常：4xx 响应、鉴权失败、限流、单次通道发送失败与重试、解密失败 |
| `info` | 生命周期与业务节点：启动配置、投递开始/完成、登录、时区变更、清理任务 |
| `debug` | 排查细节：每条 SQL、通道发送的请求构造与响应、单通道成功 |

一次请求产生的日志可以用 `request_id` 串起来：响应头 `X-Request-ID` 与日志里的
`request_id` 字段是同一个值（客户端自带该请求头时原样沿用，便于对接网关追踪）。
请求日志还带 `route`（路由模板，便于聚合）、`user_id` / `token_id`（调用方身份）
与 `latency`、`bytes`；失败请求的原因由单独一条 `请求返回业务错误` 记录，
含 `error_code` / `error_message` / `field`。

::: tip 排查投递问题
交付日志页里能看到通道级的错误文案；服务端日志里同一个 `log=<投递日志 ID>` 的
`通道发送失败` 记录则给出每次尝试的 `permanent`、`attempt` 与耗时，两者可以对照看。
:::

`MISS_TOLERANCE_MINUTES` 需要跟你的使用习惯匹配：如果希望「哪怕服务器宕机半天，恢复后仍要收到提醒」，
把它调大；如果希望「过期的提醒就别吵我了」，保持默认即可。

## 站点配置

面板**设置**页里可以修改的配置存在数据库中，改完立即生效，不需要重启：

| 键 | 说明 |
|---|---|
| `app_name` | 站点名称，显示在侧边栏与浏览器标题 |
| `logo_svg` | 自定义 Logo 的 SVG 源码 |
| `timezone` | 站点时区（如 `Asia/Shanghai`）。保存后立即生效，所有启用提醒按新时区重算下次触发时间并写回数据库。值未变化时不触发重建 |

值为空字符串时表示删除该配置项，回退到内置默认（`timezone` 回退到 `Asia/Shanghai`）。

`/api/config` 是一组**白名单**接口：只接受上表中的三个键，写入其它键（包括 `ai_` 前缀）
一律返回 `400 validation_failed`，读取时也只返回这三个键。这样通用配置接口无法绕过
AI 设置的加密与校验路径去写密钥。详见[内置 API](/reference/internal-api#api-config-的白名单)。

AI 助手相关配置也以 `ai_` 前缀存在同一张表里，但**不通过 `/api/config` 读写**，
而是走专门的 `GET/PUT /api/ai/settings`：

| 键 | 说明 |
|---|---|
| `ai_enabled` | 是否启用 AI 助手 |
| `ai_provider` | `anthropic` / `openai`（OpenAI 兼容协议） |
| `ai_base_url` | 厂商地址；Anthropic 留空走官方 |
| `ai_model` | 模型名 |
| `ai_api_key_enc` | 加密后的 API Key，读出时恒显示 `***` |
| `ai_custom_prompt` | 用户自定义的追加提示词，拼接在内置系统提示词之后 |
| `ai_max_retries` | 接口失败后的最大重试次数，默认 `10`，范围 0–100（0 表示不重试） |

密钥单独走 AI 设置接口，是为了避免通用配置接口把明文或密文带出面板以外的地方。
详见[AI 助手](/guide/ai-assistant)。

::: tip 没有新增环境变量
AI 助手的全部配置都在面板里改，改完立即生效（每次对话都按最新设置构造客户端），
不需要重启进程，也不需要额外的环境变量。

对话内容不写入数据库；`config` 表里的 `ai_*` 只保存厂商、模型与加密后的密钥。
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
      - BASE_URL=https://reminder.example.com
      - TIMEZONE=Asia/Shanghai
```
