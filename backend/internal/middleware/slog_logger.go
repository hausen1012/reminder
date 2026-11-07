package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// slowRequestThreshold 是「成功但慢」的请求被提升为 Warn 的阈值。
//
// 2xx 默认降到 Debug（见 SlogLogger），但一个成功的请求也可能值得注意：
// 秒级以上的响应通常意味着慢查询、外部通道超时或锁竞争。
// 这类请求保持可见，否则调高级别排查时会把它们一起藏掉。
const slowRequestThreshold = time.Second

// SlogLogger 返回基于 slog 的 Gin 请求日志中间件。
//
// 替换 gin.Default() 自带的日志记录器，统一使用结构化日志。
//
// 级别按状态码与耗时决定，而不是一律 Info：
//   - 5xx → Error：请求失败到服务端问题，必须能一眼筛出来；
//   - 4xx → Warn：客户端问题（参数错、鉴权失败、限流），排查时同样有用；
//   - 2xx/3xx → Debug：绝大多数是面板轮询与页面加载，Info 级别下会淹没有用信息；
//     耗时超过 1 秒的仍然提到 Warn。
//
// 本中间件只负责「访问日志」这一件事（谁、什么方法、打到哪、多久、多大、什么状态）；
// 失败原因由 ErrorHandler 单独记录，两处共用 request_id，可以按 id 串起来看。
func SlogLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		// 没有错误的成功请求在 Info 级别下不输出；先判级别，
		// 避免为一条最终会被丢弃的日志拼装字段。
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
			attrs = append(attrs, slog.String("query", query))
		}
		if size := c.Writer.Size(); size > 0 {
			attrs = append(attrs, slog.Int("bytes", size))
		}

		slog.LogAttrs(c.Request.Context(), level, "request", attrs...)
	}
}
