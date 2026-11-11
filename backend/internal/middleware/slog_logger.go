package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/reminder/backend/internal/textutil"
)

// slowRequestThreshold 是「成功但慢」的请求被提升为 Warn 的阈值。
const slowRequestThreshold = time.Second

// SlogLogger 记录 Gin 请求日志（5xx=Error，4xx/慢请求=Warn，其余=Debug）。
func SlogLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		// 成功请求在 Info 级别不输出；先判级别，避免白拼字段
		level := slog.LevelDebug
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		case latency >= slowRequestThreshold:
			level = slog.LevelWarn
		}
		if !slog.Default().Enabled(c.Request.Context(), level) {
			return
		}

		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", status),
			slog.Duration("latency", latency.Round(time.Millisecond)),
			slog.String("ip", c.ClientIP()),
		}
		// 路由模板比原始 path 更适合聚合统计（/api/reminders/12 → /api/reminders/:id）
		if route := c.FullPath(); route != "" && route != path {
			attrs = append(attrs, slog.String("route", route))
		}
		if requestID := RequestIDOf(c); requestID != "" {
			attrs = append(attrs, slog.String("request_id", requestID))
		}
		if userID, ok := c.Get("user_id"); ok {
			attrs = append(attrs, slog.Any("user_id", userID))
		}
		if tokenID, ok := c.Get("token_id"); ok {
			attrs = append(attrs, slog.Any("token_id", tokenID))
		}
		if query != "" {
			// query 里可能带令牌或一次性凭据，只保留参数名
			attrs = append(attrs, slog.String("query", textutil.RedactQuery(query)))
		}
		if size := c.Writer.Size(); size > 0 {
			attrs = append(attrs, slog.Int("bytes", size))
		}

		slog.LogAttrs(c.Request.Context(), level, "request", attrs...)
	}
}
