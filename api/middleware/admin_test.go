package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 批量删除接口挂在 Auth + AdminOnly 之后：无 Key 401，普通用户 403，管理员放行。
func TestAdminOnlyGuardsBatchDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := fakeLookup{"user": {APIKey: "user"}, "admin": {APIKey: "admin", IsAdmin: true}}
	r := gin.New()
	g := r.Group("/api", Auth(store))
	g.Use(AdminOnly())
	g.POST("/admin/accounts/batch-delete", func(c *gin.Context) { c.Status(http.StatusOK) })
	for key, want := range map[string]int{"": 401, "user": 403, "admin": 200} {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/accounts/batch-delete", strings.NewReader(`{"ids":[]}`))
		if key != "" {
			req.Header.Set("X-API-Key", key)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Errorf("key %q: got %d want %d", key, w.Code, want)
		}
		if want == 403 && !strings.Contains(w.Body.String(), "需要管理员权限") {
			t.Errorf("403 message: %s", w.Body.String())
		}
	}
}
