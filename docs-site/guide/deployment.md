# 部署

## 形态选择

| 形态 | 适用场景 | 需要什么 |
|---|---|---|
| 单文件二进制 | 一台自己的服务器 / 树莓派 / NAS | 目标平台的可执行文件 |
| Docker | 已有 Docker 环境 | `docker-compose` |

两种形态的运行逻辑一致：一个进程、一个 SQLite 文件、一个端口。**不支持多副本部署**，
因为调度器与限流器都是进程内状态。

## 单文件二进制

```bash
make build              # 当前平台
make build-linux        # linux/amd64
make build-macos        # darwin/amd64
make build-windows      # windows/amd64
```

产物在 `build/` 目录下，是一个自包含的可执行文件（前端已嵌入）。拷到目标机器后：

```bash
PORT=8765 \
DB_PATH=/var/lib/reminder/reminder.db \
BASE_URL=https://reminder.example.com \
JWT_SECRET='随机串' \
ENCRYPTION_KEY='base64编码的32字节' \
LOG_FILE=/var/log/reminder \
./server
```

跨平台构建需要交叉编译。由于 SQLite 驱动使用 CGO，交叉编译时**构建机器**上需要有目标平台的 C 交叉工具链；
配置麻烦时直接用 Docker 部署更省事。

## Docker

```bash
docker compose up -d --build
```

`docker-compose.yml` 已经把数据库与日志挂到宿主机目录（`./reminder/db`、`./reminder/log`），
容器重建不会丢数据。所有环境变量都可以通过宿主机环境或 `.env` 文件覆盖。

镜像构建分三段：Node 构建前端 → Go 编译（前端产物拷进 `cmd/server/web` 供 `go:embed`）→
Alpine 运行。最终镜像只包含二进制与时区数据。

打 tag 时 GitHub Actions 会自动构建并推送到 Docker Hub（见 `.github/workflows/docker-release.yml`）。

## 反向代理

确认链接是明文 HTTP 路径 `/c/:token`，需要能被手机访问。用 Nginx 时至少要：

```nginx
location / {
    proxy_pass http://127.0.0.1:8765;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

然后把 `BASE_URL` 改成外部可访问的域名。系统本身不生成绝对路径跳转，因此不需要额外配置。

## 备份

数据全部在一个 SQLite 文件里。备份方式是停服务后复制文件（连同 `-wal` / `-shm`），
或者用 SQLite 的在线备份接口：

```bash
sqlite3 /var/lib/reminder/reminder.db ".backup '/backup/reminder-$(date +%F).db'"
```

日志表可能很大，如果只需要提醒配置，可以在备份前先清理日志。

## 健康检查

```bash
curl http://localhost:8765/api/health
```

返回数据库连通性、提醒与日志条数、调度器注册数量与上次触发时间、兜底扫描是否运行中。
这个接口不需要认证，可以直接给监控系统用。

## 升级

1. 备份数据库文件
2. 替换可执行文件 / 重新构建镜像
3. 重启进程

启动时会自动执行数据库迁移，新增的表与字段会自动补齐，不需要手工执行 SQL。
