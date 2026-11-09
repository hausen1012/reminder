# Stage 1: Frontend build
FROM node:20-alpine AS frontend
WORKDIR /app
COPY frontend/ .
RUN npm ci && npm run build

# Stage 2: Go build
FROM golang:1.25-alpine AS backend
WORKDIR /app
RUN apk add --no-cache gcc musl-dev
ENV CGO_ENABLED=1
COPY backend/ .
# The frontend build output must go to backend/cmd/server/web/ because
# the //go:embed web/* directive in main.go resolves relative to cmd/server/
COPY --from=frontend /app/dist ./cmd/server/web

# 版本号与构建时间注入，关于页据此展示当前版本并判断是否有更新。
# 版本号由构建参数传入（CI 用 tag 名），本地构建默认 dev。
ARG VERSION=dev
ARG BUILD_TIME=
RUN BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" && \
    go build \
      -ldflags "-X github.com/reminder/backend/internal/version.Version=$VERSION -X github.com/reminder/backend/internal/version.BuildTime=$BUILD_TIME" \
      -o server ./cmd/server

# Stage 3: Runtime
FROM alpine:3.19
# tzdata 必须安装：站点时区（设置页可改）与 TIMEZONE 都依赖时区数据库
RUN apk --no-cache add tzdata
WORKDIR /app
COPY --from=backend /app/server .
EXPOSE 8765
CMD ["./server"]
