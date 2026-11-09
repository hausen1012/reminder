
.PHONY: dev dev-backend dev-frontend build build-frontend build-linux build-macos build-windows docker clean test lint fmt docs docs-build

# 一键启动前后端开发环境（依赖 bash；Windows 图形终端可直接运行 dev.ps1）
dev:
	./dev.sh

# 开发默认开 debug：本地只看得到 SQL、访问日志与投递中间步骤才方便排查。
# 想临时降噪：make dev-backend LOG_LEVEL=info
dev-backend:
	cd backend && LOG_LEVEL=$${LOG_LEVEL:-debug} LOG_FILE=../logs DB_PATH=../data/reminder.db go run ./cmd/server/

dev-frontend:
	cd frontend && npm run dev

# Build frontend + copy to backend for Go embedding
# The embed path //go:embed web/* in cmd/server/main.go resolves relative to cmd/server/
#
# 先清空 web/ 再拷贝：go:embed 会把目录内所有文件打进二进制，
# 不清空时上一次构建残留的带 hash 资源会一直累积，二进制只增不减。
# .gitkeep 要保留——CI 依赖它让目录在空仓库里也能编译。
build-frontend:
	cd frontend && npm run build
	mkdir -p backend/cmd/server/web
	find backend/cmd/server/web -mindepth 1 ! -name '.gitkeep' -exec rm -rf {} +
	cp -r frontend/dist/* backend/cmd/server/web/

# 版本号注入：取自最近的 git tag，仓库外或无 tag 时为 dev。
# 构建时间用 UTC，前端按站点时区展示。
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -X github.com/reminder/backend/internal/version.Version=$(VERSION) \
          -X github.com/reminder/backend/internal/version.BuildTime=$(BUILD_TIME)

# Build backend binary (local OS)
build: build-frontend
	cd backend && go build -ldflags "$(LDFLAGS)" -o ../build/server ./cmd/server/

# Cross-platform builds
build-linux: build-frontend
	cd backend && GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ../build/server-linux ./cmd/server/

build-macos: build-frontend
	cd backend && GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ../build/server-macos ./cmd/server/

build-windows: build-frontend
	cd backend && GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ../build/server.exe ./cmd/server/

# Docker
docker:
	docker-compose up --build

# 质量门禁：格式检查 + 静态检查 + 单元测试
fmt:
	cd backend && gofmt -w .

lint:
	cd backend && go vet ./...
	@cd backend && test -z "$$(gofmt -l .)" || (echo "以下文件未格式化，请运行 make fmt:"; gofmt -l .; exit 1)

test:
	cd backend && go test ./...
	cd frontend && npm test

# 文档站点：本地预览（热更新）
docs:
	cd docs-site && npm run docs:dev

# 文档站点：构建静态文件到 docs-site/.vitepress/dist
docs-build:
	cd docs-site && npm run docs:build

# Clean
clean:
	rm -rf frontend/dist backend/cmd/server/web/* build docs-site/.vitepress/dist
