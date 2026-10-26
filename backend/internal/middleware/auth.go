package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// jwtClaims 是 JWT 中我们关心的字段。
//
// 用结构化 claims 而不是 MapClaims：解析即校验类型，避免手写
// float64 断言后出现"类型不符静默变 0"的情况。
type jwtClaims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// JWTAuth 返回基于 HS256 的认证中间件。
//
// 校验失败一律返回 401；验签算法通过 jwt.WithValidMethods 限定为 HS256，
// 从而阻断 alg=none 与算法混淆攻击。
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

// abortUnauthorized 以统一格式结束请求。
func abortUnauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"code":    401,
		"message": message,
		"data":    nil,
	})
}
