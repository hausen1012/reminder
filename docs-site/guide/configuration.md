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
| `ENCRYPTION_KEY` | 内置 fallback | 通道敏感字段（SMTP 密码、钉钉加签密钥、Webhook Authorization）的加密密钥 |

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
生产环境务必显式配置。

::: danger 换密钥会导致旧数据读不出来
通道配置以密文入库。更换 `ENCRYPTION_KEY` 后，已有通道的敏感字段将无法解密，
需要到**通知**页把对应字段重新填一遍。
:::

### 链接与时间

| 变量 | 默认值 | 说明 |
|---|---|---|
| `BASE_URL` | `http://localhost:8765` | 对外可访问的根地址，用于拼接确认链接 `/c/:token` 与日志里的确认地址 |
| `TIMEZONE` | `Asia/Shanghai` | 系统时区，影响触发计算与展示；解析失败时回退到 `Asia/Shanghai` |

`BASE_URL` 必须填**用户手机上能访问到的地址**。如果只在局域网里自用，填内网 IP；
否则确认链接会指向 `localhost`，手机上点开是空的。

### 调度与日志

| 变量 | 默认值 | 说明 |
|---|---|---|
| `SWEEP_INTERVAL_SEC` | `60` | 兜底扫描间隔（秒）。扫描用于补发进程重启期间错过的触发 |
| `MISS_TOLERANCE_MINUTES` | `60` | 漏触发容忍窗口（分钟）。早于该窗口的漏触发直接标记为 `expired`，不再补发 |
| `LOG_AUTO_PURGE_DAYS` | `0` | 日志自动清理保留天数；`0` 表示不自动清理。大于 0 时每天凌晨 3 点清理一次 |
| `LOG_FILE` | `/var/log/reminder` | 日志目录。目录创建失败时仅输出到标准错误 |

`MISS_TOLERANCE_MINUTES` 需要跟你的使用习惯匹配：如果希望「哪怕服务器宕机半天，恢复后仍要收到提醒」，
把它调大；如果希望「过期的提醒就别吵我了」，保持默认即可。

## 站点配置

面板**设置**页里可以修改的配置存在数据库中，改完立即生效，不需要重启：

| 键 | 说明 |
|---|---|
| `app_name` | 站点名称，显示在侧边栏与浏览器标题 |
| `logo_svg` | 自定义 Logo 的 SVG 源码 |

值为空字符串时表示删除该配置项，回退到内置默认。

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
