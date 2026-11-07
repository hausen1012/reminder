#!/bin/bash
# 快速启动开发服务器，后端后台运行，前端前台运行（热更新）
# Ctrl+C 停止所有服务
set -e

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"

check_deps() {
  if ! command -v go &>/dev/null; then
    echo "Error: Go not found, install from https://go.dev" >&2
    exit 1
  fi
  if ! command -v node &>/dev/null; then
    echo "Error: Node.js not found, install from https://nodejs.org" >&2
    exit 1
  fi
}

install_deps() {
  if [ ! -d "$ROOT_DIR/frontend/node_modules" ]; then
    echo "Installing frontend dependencies..."
    cd "$ROOT_DIR/frontend" && npm install
  fi
}

cleanup() {
  echo ""
  echo "Stopping services..."
  if [ -n "$BACKEND_PID" ]; then
    kill "$BACKEND_PID" 2>/dev/null || true
    wait "$BACKEND_PID" 2>/dev/null || true
  fi
  echo "Stopped."
  exit 0
}

check_deps
install_deps
trap cleanup SIGINT SIGTERM

echo "Starting backend (Go/Gin)..."
cd "$ROOT_DIR/backend"
# 开发默认开 debug：本地只看得到 SQL、访问日志与投递中间步骤才方便排查。
# 想临时降噪就 `LOG_LEVEL=info ./dev.sh`（外部值优先于这里的默认值）。
LOG_LEVEL="${LOG_LEVEL:-debug}" \
LOG_FILE="$ROOT_DIR/logs" DB_PATH="$ROOT_DIR/data/reminder.db" go run ./cmd/server/ &
BACKEND_PID=$!

echo "Waiting for backend..."
for i in $(seq 1 30); do
  if curl -s http://localhost:8765/api/health >/dev/null 2>&1; then
    echo "Backend ready (PID: $BACKEND_PID)"
    break
  fi
  if [ $i -eq 30 ]; then
    echo "Warning: backend startup timeout" >&2
  fi
  sleep 1
done

echo "Starting frontend (Vite)..."
cd "$ROOT_DIR/frontend"
npx vite --host

cleanup