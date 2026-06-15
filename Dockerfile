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
RUN go build -o server ./cmd/server

# Stage 3: Runtime
FROM alpine:3.19
RUN apk --no-cache add tzdata
WORKDIR /app
COPY --from=backend /app/server .
EXPOSE 8765
CMD ["./server"]
