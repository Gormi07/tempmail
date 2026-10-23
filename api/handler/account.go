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

	"github.com/google/uuid"

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
		respondError(c, http.StatusBadRequest, MsgBadRequest, err)
		return
	}

	account, err := h.store.GetAccountByAPIKey(c.Request.Context(), strings.TrimSpace(req.APIKey))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": middleware.AuthErrorMessage(err)})
		return
	}
	if keyLogin == "false" && !account.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "API Key 登录已关闭"})
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
		return fmt.Errorf("生成数量需在 1 到 %d 之间", MaxBatchCreate)
	}
	if r.Username != "" && (len(r.Username) < 2 || len(r.Username) > 48) {
		return errors.New("用户名需为 2-48 个字符")
	}
	if r.Validity < 0 || r.Validity > 3650 {
		return errors.New("有效期需在 0 到 3650 之间")
	}
	if r.ValidityUnit == "" {
		r.ValidityUnit = "day"
	}
	if r.ValidityUnit != "day" && r.ValidityUnit != "month" {
		return errors.New("有效期单位只能是天（day）或月（month）")
	}
	return nil
}

// POST /api/admin/accounts - 创建账号 / 批量生成 API Key（管理员）
func (h *AccountHandler) Create(c *gin.Context) {
	var req CreateAccountsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, MsgBadRequest, err)
		return
	}
	if err := req.Normalize(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}) // 自定义中文校验信息
		return
	}
	expiresAt, err := model.ComputeExpiry(time.Now(), req.Validity, req.ValidityUnit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}) // 自定义中文校验信息
		return
	}

	accounts, err := h.store.CreateAccountsBatch(c.Request.Context(), req.Username, req.Count, expiresAt)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			respondError(c, http.StatusConflict, "用户名已被占用，请换一个", err)
			return
		}
		respondError(c, http.StatusInternalServerError, "创建账户失败，请稍后再试", err)
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

	filter := store.AccountFilter{Query: c.Query("q"), Status: c.Query("status")}
	if !store.ValidAccountStatus(filter.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "状态筛选无效，只能是全部、有效、已过期或永久"})
		return
	}
	if len([]rune(filter.Query)) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "搜索内容过长，最多 100 个字符"})
		return
	}

	accounts, total, err := h.store.ListAccountsFiltered(c.Request.Context(), filter, page, size)
	if err != nil {
		respondError(c, http.StatusInternalServerError, MsgInternal, err)
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "账户 ID 无效"})
		return
	}

	if err := h.store.DeleteAccount(c.Request.Context(), id); err != nil {
		if errors.Is(err, store.ErrAdminAccountProtected) {
			c.JSON(http.StatusForbidden, gin.H{"error": "管理员账户不可删除"})
			return
		}
		respondError(c, http.StatusInternalServerError, MsgInternal, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "账户已删除"})
}

// MaxBatchDelete 单次批量删除上限。
const MaxBatchDelete = 500

// ParseBatchDeleteIDs 校验并去重批量删除的账户 ID。
func ParseBatchDeleteIDs(raw []string) ([]uuid.UUID, error) {
	if len(raw) == 0 {
		return nil, errors.New("请至少选择一个账户")
	}
	if len(raw) > MaxBatchDelete {
		return nil, fmt.Errorf("单次最多删除 %d 个账户", MaxBatchDelete)
	}
	seen := make(map[uuid.UUID]bool, len(raw))
	ids := make([]uuid.UUID, 0, len(raw))
	for _, r := range raw {
		id, err := parseUUID(strings.TrimSpace(r))
		if err != nil {
			return nil, errors.New("账户 ID 无效")
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// POST /api/admin/accounts/batch-delete - 批量删除账户（管理员）
// body: {"ids": ["uuid", ...]}
func (h *AccountHandler) BatchDelete(c *gin.Context) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, MsgBadRequest, err)
		return
	}
	ids, err := ParseBatchDeleteIDs(req.IDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()}) // 自定义中文校验信息
		return
	}
	if me := middleware.GetAccount(c); me != nil {
		for _, id := range ids {
			if id == me.ID {
				c.JSON(http.StatusForbidden, gin.H{"error": "不能删除当前登录的账户"})
				return
			}
		}
	}
	n, err := h.store.DeleteAccounts(c.Request.Context(), ids)
	if err != nil {
		if errors.Is(err, store.ErrAdminAccountProtected) {
			c.JSON(http.StatusForbidden, gin.H{"error": "所选账户中包含管理员账户，管理员账户不可删除"})
			return
		}
		respondError(c, http.StatusInternalServerError, "批量删除失败，请稍后再试", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("已删除 %d 个账户", n), "deleted": n})
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
