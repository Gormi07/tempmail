package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tempmail/model"

	"github.com/gin-gonic/gin"
)

type fakeLookup map[string]*model.Account

func (f fakeLookup) GetAccountByAPIKey(_ context.Context, k string) (*model.Account, error) {
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
	cases := map[string]int{"perm": 200, "valid": 200, "expired": 401, "edge": 401, "nope": 401, "": 401}
	for key, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Errorf("key %q: got %d want %d (%s)", key, w.Code, want, w.Body.String())
		}
	}
}
