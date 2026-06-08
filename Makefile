
  - 用户一旦确认，整条链立即停止.PHONY: dev dev-backend dev-frontend build build-linux build-macos build-windows docker clean

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

# Clean
clean:
	rm -rf frontend/dist backend/cmd/server/web/* build
