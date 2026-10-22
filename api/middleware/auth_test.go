package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tempmail/model"

	"github.com/gin-gonic/gin"
)

type fakeLookup map[string]*model.Account

func (f fakeLookup) GetAccountByAPIKey(_ context.Context, k string) (*model.Account, error) {
	if k == "expired-db" {
		return nil, model.ErrAPIKeyExpired
	}
	if a, ok := f[k]; ok {
		return a, nil
	}
	return nil, errors.New("not found")
}

func TestAuthExpiry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixed := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return fixed }
	defer func() { now = time.Now }()
	past, future := fixed.Add(-time.Second), fixed.Add(time.Hour)
	store := fakeLookup{
		"perm":    {APIKey: "perm"},
		"valid":   {APIKey: "valid", ExpiresAt: &future},
		"expired": {APIKey: "expired", ExpiresAt: &past},
		"edge":    {APIKey: "edge", ExpiresAt: &fixed},
	}
	r := gin.New()
	r.GET("/x", Auth(store), func(c *gin.Context) { c.Status(http.StatusOK) })
	type exp struct {
		code int
		msg  string
	}
	cases := map[string]exp{
		"perm": {200, ""}, "valid": {200, ""},
		"expired": {401, "Key已过期"}, "edge": {401, "Key已过期"}, "expired-db": {401, "Key已过期"},
		"nope": {401, "Key错误"}, "": {401, ""},
	}
	for key, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want.code {
			t.Errorf("key %q: got %d want %d (%s)", key, w.Code, want.code, w.Body.String())
		}
		if want.msg != "" && !strings.Contains(w.Body.String(), `"error":"`+want.msg+`"`) {
			t.Errorf("key %q: body %s, want message %s", key, w.Body.String(), want.msg)
		}
	}
}
