
.PHONY: dev dev-backend dev-frontend build build-frontend build-linux build-macos build-windows docker clean test lint fmt

# 一键启动前后端开发环境（依赖 bash；Windows 图形终端可直接运行 dev.ps1）
dev:
	./dev.sh

# Local development
dev-backend:
	cd backend && go run ./cmd/server/

dev-frontend:
	cd frontend && npm run dev

# Build frontend + copy to backend for Go embedding
# The embed path //go:embed web/* in cmd/server/main.go resolves relative to cmd/server/
build-frontend:
	cd frontend && npm run build
	mkdir -p backend/cmd/server/web
	cp -r frontend/dist/* backend/cmd/server/web/

# Build backend binary (local OS)
build: build-frontend
	cd backend && go build -o ../build/server ./cmd/server/

# Cross-platform builds
build-linux: build-frontend
	cd backend && GOOS=linux GOARCH=amd64 go build -o ../build/server-linux ./cmd/server/

build-macos: build-frontend
	cd backend && GOOS=darwin GOARCH=amd64 go build -o ../build/server-macos ./cmd/server/

build-windows: build-frontend
	cd backend && GOOS=windows GOARCH=amd64 go build -o ../build/server.exe ./cmd/server/

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

# Clean
clean:
	rm -rf frontend/dist backend/cmd/server/web/* build
