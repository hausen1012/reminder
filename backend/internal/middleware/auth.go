package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/reminder/backend/internal/logger"
)

// jwtClaims 是 JWT 中我们关心的字段；用结构化 claims 而非 MapClaims 以避免类型断言静默变 0。
type jwtClaims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// JWTAuth 返回基于 HS256 的认证中间件，校验失败一律 401；
// 限定 HS256 可阻断 alg=none 与算法混淆攻击。
func JWTAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			abortUnauthorized(c, "未提供认证令牌")
			return
		}

		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		claims := &jwtClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
		if err != nil || !token.Valid {
			// 对外文案相同，日志里要区分验签失败、过期还是格式错误
			if err != nil {
				logger.FromContext(c.Request.Context()).Warn("JWT 校验失败",
					"path", c.Request.URL.Path, "error", err)
			} else {
				logger.FromContext(c.Request.Context()).Warn("JWT 校验失败",
					"path", c.Request.URL.Path, "reason", "token.Valid=false")
			}
			abortUnauthorized(c, "认证令牌无效或已过期")
			return
		}

		// 缺少 user_id 的令牌视为非法：正常登录必然带上该字段。
		if claims.UserID == 0 {
			abortUnauthorized(c, "认证令牌无效")
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}

// abortUnauthorized 以统一错误格式结束请求。
func abortUnauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized,
		ErrorBody(CodeUnauthorized, message, ""))
}
