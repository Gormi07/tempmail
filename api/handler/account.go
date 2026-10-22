package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tempmail/middleware"
	"tempmail/model"
	"tempmail/store"

	"github.com/gin-gonic/gin"
)

type AccountHandler struct {
	store *store.Store
}

func NewAccountHandler(s *store.Store) *AccountHandler {
	return &AccountHandler{store: s}
}

// POST /public/key-login - API Key 登录入口，可由后台开关关闭。
func (h *AccountHandler) KeyLogin(c *gin.Context) {
	keyLogin, err := h.store.GetSetting(c.Request.Context(), "key_login_enabled")

	var req struct {
		APIKey string `json:"api_key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	account, err := h.store.GetAccountByAPIKey(c.Request.Context(), strings.TrimSpace(req.APIKey))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid api_key"})
		return
	}
	if keyLogin == "false" && !account.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "API Key login is disabled"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"api_key":    account.APIKey,
		"id":         account.ID,
		"username":   account.Username,
		"is_admin":   account.IsAdmin,
		"created_at": account.CreatedAt,
		"expires_at": account.ExpiresAt,
	})
}

// MaxBatchCreate 单次批量创建 API Key 的上限。
const MaxBatchCreate = 500

// CreateAccountsReq 创建账户 / 批量生成 API Key 请求。
//   - username: count=1 时为用户名；count>1 时作为前缀；可为空（自动生成）
//   - count: 生成数量，默认 1，最大 MaxBatchCreate
//   - validity + validity_unit(day|month): 有效期；validity<=0 或缺省表示永久
type CreateAccountsReq struct {
	Username     string `json:"username"`
	Count        int    `json:"count"`
	Validity     int    `json:"validity"`
	ValidityUnit string `json:"validity_unit"`
}

// Normalize 校验并补全默认值。
func (r *CreateAccountsReq) Normalize() error {
	r.Username = strings.TrimSpace(r.Username)
	if r.Count == 0 {
		r.Count = 1
	}
	if r.Count < 1 || r.Count > MaxBatchCreate {
		return fmt.Errorf("count must be between 1 and %d", MaxBatchCreate)
	}
	if r.Username != "" && (len(r.Username) < 2 || len(r.Username) > 48) {
		return errors.New("username must be 2-48 characters")
	}
	if r.Validity < 0 || r.Validity > 3650 {
		return errors.New("validity must be between 0 and 3650")
	}
	if r.ValidityUnit == "" {
		r.ValidityUnit = "day"
	}
	if r.ValidityUnit != "day" && r.ValidityUnit != "month" {
		return errors.New("validity_unit must be day or month")
	}
	return nil
}

// POST /api/admin/accounts - 创建账号 / 批量生成 API Key（管理员）
func (h *AccountHandler) Create(c *gin.Context) {
	var req CreateAccountsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := req.Normalize(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	expiresAt, err := model.ComputeExpiry(time.Now(), req.Validity, req.ValidityUnit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	accounts, err := h.store.CreateAccountsBatch(c.Request.Context(), req.Username, req.Count, expiresAt)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists or db error: " + err.Error()})
		return
	}

	items := make([]gin.H, 0, len(accounts))
	for _, a := range accounts {
		items = append(items, gin.H{"id": a.ID, "username": a.Username, "api_key": a.APIKey, "expires_at": a.ExpiresAt})
	}
	first := accounts[0]
	// 保留旧字段（id/username/api_key）以兼容单个创建的调用方。
	c.JSON(http.StatusCreated, gin.H{
		"id":         first.ID,
		"username":   first.Username,
		"api_key":    first.APIKey,
		"expires_at": first.ExpiresAt,
		"count":      len(accounts),
		"accounts":   items,
	})
}

// GET /api/admin/accounts - 列出所有账号（管理员）
func (h *AccountHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	accounts, total, err := h.store.ListAccounts(c.Request.Context(), page, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  accounts,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// DELETE /api/admin/accounts/:id - 删除账号（管理员）
func (h *AccountHandler) Delete(c *gin.Context) {
	id, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account id"})
		return
	}

	if err := h.store.DeleteAccount(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "account deleted"})
}

// GET /api/me - 查看当前账号信息
func (h *AccountHandler) Me(c *gin.Context) {
	account := middleware.GetAccount(c)
	c.JSON(http.StatusOK, gin.H{
		"id":         account.ID,
		"username":   account.Username,
		"is_admin":   account.IsAdmin,
		"created_at": account.CreatedAt,
		"expires_at": account.ExpiresAt,
	})
}
