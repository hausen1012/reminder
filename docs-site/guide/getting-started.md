# 快速开始

## 用 Docker 运行

最快的方式，免安装 Go 与 Node.js：

```bash
docker compose up -d --build
```

容器会监听 `8765` 端口，数据与日志挂载在仓库目录下的 `reminder/db` 与 `reminder/log`。
打开 `http://localhost:8765`，用初始账号登录即可（默认 `admin` / `admin123`，见[配置项](/guide/configuration)）。

生产环境请务必通过环境变量覆盖初始账号与密钥，参考[部署](/guide/deployment)。

## 从源码构建

需要 Go 1.25 与 Node.js 20 以上。

```bash
# 构建前端并把产物嵌入后端，输出 build/server
make build

# 直接运行
./build/server          # Windows 为 build\server.exe
```

首次启动会在 `DB_PATH` 指定的位置初始化 SQLite 数据库并创建管理员账号。

## 本地开发

仓库提供一键启动脚本，后端与前端同时运行，前端带热更新：

```bash
make dev
```

也可以分开启动，便于单独调试：

```bash
make dev-backend    # 后端，监听 8765
make dev-frontend   # 前端 Vite dev server，监听 5173
```

开发模式下打开 `http://localhost:5173`，前端会把 `/api` 请求代理到后端。

::: tip Windows 用户
`make dev` 依赖 bash。在 Windows 图形终端里可直接运行仓库根目录的 `dev.ps1`，
它会打开两个独立窗口分别启动后端与前端。
:::

## 登录并完成第一次提醒

1. 打开面板，用管理员账号登录
2. 到**通知**页新建一个通知渠道。想先试试效果，可以选「日志输出」——它不发外部消息，
   只把内容打到服务端日志里，无需任何配置
3. 到**提醒**页新建提醒，填标题与内容，选定时间，勾上刚才的渠道
4. 保存后可以在提醒列表看到下次触发时间；也可以点「测试」用当前表单立刻试发一次

## 下一步

- [提醒与调度](/guide/reminders)：四种调度形态怎么写
- [通知渠道](/guide/channels)：每种渠道要填什么
- [确认机制](/guide/confirmation)：让提醒必须被确认
