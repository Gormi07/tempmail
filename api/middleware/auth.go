package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"tempmail/model"

	"github.com/gin-gonic/gin"
)

const AccountKey = "account"

// AccountLookup 是认证所需的最小存储接口（*store.Store 已实现）。
type AccountLookup interface {
	GetAccountByAPIKey(ctx context.Context, apiKey string) (*model.Account, error)
}

// now 便于测试替换。
var now = time.Now

// Auth API Key 认证中间件。过期的 Key（expires_at <= now）一律拒绝。
func Auth(s AccountLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("Authorization")
		if apiKey == "" {
			apiKey = c.GetHeader("X-API-Key")
		}
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}

		// 支持 Bearer token 格式
		apiKey = strings.TrimPrefix(apiKey, "Bearer ")
		apiKey = strings.TrimSpace(apiKey)

		if apiKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing api_key: use Authorization, X-API-Key, or ?api_key= query param",
			})
			return
		}

		account, err := s.GetAccountByAPIKey(c.Request.Context(), apiKey)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": AuthErrorMessage(err)})
			return
		}

		// SQL 已过滤过期账户，这里再做一次校验作为兜底。
		if account.IsExpired(now()) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": model.MsgKeyExpired,
			})
			return
		}

		c.Set(AccountKey, account)
		c.Next()
	}
}

// AuthErrorMessage 把查找 Key 的错误映射为面向用户的提示：过期 → 「Key已过期」，其余 → 「Key错误」。
func AuthErrorMessage(err error) string {
	if errors.Is(err, model.ErrAPIKeyExpired) {
		return model.MsgKeyExpired
	}
	return model.MsgKeyInvalid
}

// AdminOnly 管理员权限中间件
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		account := GetAccount(c)
		if account == nil || !account.IsAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "admin access required",
			})
			return
		}
		c.Next()
	}
}

func GetAccount(c *gin.Context) *model.Account {
	val, exists := c.Get(AccountKey)
	if !exists {
		return nil
	}
	a, ok := val.(*model.Account)
	if !ok {
		return nil
	}
	return a
}
