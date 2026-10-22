package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tempmail/store"

	"github.com/gin-gonic/gin"
)

const (
	linuxDOAuthURL     = "https://connect.linux.do/oauth2/authorize"
	linuxDOTokenURL    = "https://connect.linux.do/oauth2/token"
	linuxDOUserInfoURL = "https://connect.linux.do/api/user"
	linuxDOStateCookie = "tm_linuxdo_state"
)

type LinuxDOHandler struct {
	store           *store.Store
	envClientID     string
	envClientSecret string
	envRedirectURL  string
	httpClient      *http.Client
}

func NewLinuxDOHandler(s *store.Store, clientID, clientSecret, redirectURL string) *LinuxDOHandler {
	return &LinuxDOHandler{
		store:           s,
		envClientID:     clientID,
		envClientSecret: clientSecret,
		envRedirectURL:  redirectURL,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *LinuxDOHandler) Start(c *gin.Context) {
	if !h.loginEnabled(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Linux DO 登录已关闭"})
		return
	}
	cfg := h.oauthConfig(c)
	if cfg.clientID == "" || cfg.clientSecret == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Linux DO 登录配置不完整，请联系管理员"})
		return
	}
	state, err := randomState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法发起登录，请稍后再试"})
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(linuxDOStateCookie, state, 600, "/", "", false, true)

	q := url.Values{}
	q.Set("client_id", cfg.clientID)
	q.Set("redirect_uri", cfg.redirectURL)
	q.Set("response_type", "code")
	q.Set("scope", "user")
	q.Set("state", state)
	c.Redirect(http.StatusFound, linuxDOAuthURL+"?"+q.Encode())
}

func (h *LinuxDOHandler) Callback(c *gin.Context) {
	if !h.loginEnabled(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Linux DO 登录已关闭"})
		return
	}
	cfg := h.oauthConfig(c)
	if cfg.clientID == "" || cfg.clientSecret == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Linux DO 登录配置不完整，请联系管理员"})
		return
	}
	state := c.Query("state")
	cookieState, err := c.Cookie(linuxDOStateCookie)
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(cookieState)) != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "登录状态已失效，请重新发起登录"})
		return
	}
	c.SetCookie(linuxDOStateCookie, "", -1, "/", "", false, true)

	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "授权码缺失，请重新发起登录"})
		return
	}

	token, err := h.exchangeCode(code, cfg)
	if err != nil {
		respondError(c, http.StatusBadGateway, "Linux DO 登录失败，请稍后再试", err)
		return
	}
	profile, err := h.fetchUserInfo(token)
	if err != nil {
		respondError(c, http.StatusBadGateway, "获取 Linux DO 账号信息失败，请稍后再试", err)
		return
	}
	linuxDOID := profile.IDString()
	if linuxDOID == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "未能获取 Linux DO 账号信息，请重试"})
		return
	}

	username := linuxDOUsername(profile)
	account, err := h.store.GetOrCreateLinuxDOAccount(c.Request.Context(), linuxDOID, username)
	if err != nil {
		respondError(c, http.StatusInternalServerError, MsgInternal, err)
		return
	}

	apiKeyJSON, _ := json.Marshal(account.APIKey)
	accountJSON, _ := json.Marshal(gin.H{
		"id":         account.ID,
		"username":   account.Username,
		"is_admin":   account.IsAdmin,
		"created_at": account.CreatedAt,
	})
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, fmt.Sprintf(`<!doctype html><html><body><script>
localStorage.setItem('tm_apikey', %s);
localStorage.setItem('tm_account', %s);
location.replace('/');
</script></body></html>`, string(apiKeyJSON), string(accountJSON)))
}

func (h *LinuxDOHandler) loginEnabled(c *gin.Context) bool {
	enabled, err := h.store.GetSetting(c.Request.Context(), "linuxdo_login_enabled")
	return err == nil && enabled == "true"
}

type linuxDOOAuthConfig struct {
	clientID     string
	clientSecret string
	redirectURL  string
}

func (h *LinuxDOHandler) oauthConfig(c *gin.Context) linuxDOOAuthConfig {
	clientID, _ := h.store.GetSetting(c.Request.Context(), "linuxdo_client_id")
	clientSecret, _ := h.store.GetSetting(c.Request.Context(), "linuxdo_client_secret")
	redirectURL, _ := h.store.GetSetting(c.Request.Context(), "linuxdo_redirect_url")
	if clientID == "" {
		clientID = h.envClientID
	}
	if clientSecret == "" {
		clientSecret = h.envClientSecret
	}
	if redirectURL == "" {
		redirectURL = h.envRedirectURL
	}
	if redirectURL == "" {
		redirectURL = requestBaseURL(c) + "/public/auth/linuxdo/callback"
	}
	return linuxDOOAuthConfig{clientID: clientID, clientSecret: clientSecret, redirectURL: redirectURL}
}

func requestBaseURL(c *gin.Context) string {
	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = c.Request.Host
	}
	return scheme + "://" + host
}

func (h *LinuxDOHandler) exchangeCode(code string, cfg linuxDOOAuthConfig) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", cfg.redirectURL)
	form.Set("client_id", cfg.clientID)
	form.Set("client_secret", cfg.clientSecret)

	req, err := http.NewRequest(http.MethodPost, linuxDOTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := h.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("Linux DO token exchange failed: HTTP %d", res.StatusCode)
	}
	var data struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}
	if data.AccessToken == "" {
		return "", fmt.Errorf("Linux DO token exchange returned no access_token")
	}
	return data.AccessToken, nil
}

func (h *LinuxDOHandler) fetchUserInfo(token string) (*linuxDOProfile, error) {
	req, err := http.NewRequest(http.MethodGet, linuxDOUserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := h.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("Linux DO userinfo failed: HTTP %d", res.StatusCode)
	}
	var profile linuxDOProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

type linuxDOProfile struct {
	ID       json.RawMessage `json:"id"`
	Username string          `json:"username"`
	Name     string          `json:"name"`
}

func linuxDOUsername(profile *linuxDOProfile) string {
	for _, v := range []string{profile.Username, profile.Name, "linuxdo_" + profile.IDString()} {
		v = strings.TrimSpace(v)
		if v != "" {
			return normalizeUsername(v)
		}
	}
	return "linuxdo_user"
}

func (p *linuxDOProfile) IDString() string {
	var s string
	if err := json.Unmarshal(p.ID, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	decoder := json.NewDecoder(strings.NewReader(string(p.ID)))
	decoder.UseNumber()
	if err := decoder.Decode(&n); err == nil {
		return n.String()
	}
	return strings.TrimSpace(string(p.ID))
}

func normalizeUsername(s string) string {
	return normalizeUsernameWithFallback(s, "linuxdo_user")
}

func normalizeUsernameWithFallback(s, fallback string) string {
	s = strings.ToLower(strings.TrimSpace(html.UnescapeString(s)))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	name := b.String()
	if len(name) < 2 {
		return fallback
	}
	if len(name) > 48 {
		return name[:48]
	}
	return name
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
