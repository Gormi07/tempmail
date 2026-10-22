package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRespondErrorHidesInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	respondError(c, http.StatusInternalServerError, MsgInternal, errors.New("pq: relation accounts secret detail"))
	body := w.Body.String()
	if w.Code != 500 || !strings.Contains(body, MsgInternal) {
		t.Fatalf("unexpected response %d %s", w.Code, body)
	}
	if strings.Contains(body, "secret detail") || strings.Contains(body, "pq:") {
		t.Fatalf("internal error leaked: %s", body)
	}
}
