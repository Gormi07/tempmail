package handler

import (
	"net/http"
	"strconv"

	"tempmail/middleware"
	"tempmail/store"

	"github.com/gin-gonic/gin"
)

type EmailHandler struct {
	store *store.Store
}

func NewEmailHandler(s *store.Store) *EmailHandler {
	return &EmailHandler{store: s}
}

// GET /api/mailboxes/:mailbox_id/emails - 列出邮件
func (h *EmailHandler) List(c *gin.Context) {
	account := middleware.GetAccount(c)
	mailboxID, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱 ID 无效"})
		return
	}

	// 验证邮箱归属
	_, err = h.store.GetMailbox(c.Request.Context(), mailboxID, account.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "邮箱不存在或已过期"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	emails, total, err := h.store.ListEmails(c.Request.Context(), mailboxID, page, size)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取邮件失败，请稍后再试", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  emails,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// GET /api/mailboxes/:mailbox_id/emails/:id - 读取邮件内容
func (h *EmailHandler) Get(c *gin.Context) {
	account := middleware.GetAccount(c)
	mailboxID, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱 ID 无效"})
		return
	}

	// 验证邮箱归属
	_, err = h.store.GetMailbox(c.Request.Context(), mailboxID, account.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "邮箱不存在或已过期"})
		return
	}

	emailID, err := parseUUID(c.Param("email_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮件 ID 无效"})
		return
	}

	email, err := h.store.GetEmail(c.Request.Context(), emailID, mailboxID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "邮件不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"email": email})
}

// DELETE /api/mailboxes/:mailbox_id/emails/:id - 删除邮件
func (h *EmailHandler) Delete(c *gin.Context) {
	account := middleware.GetAccount(c)
	mailboxID, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱 ID 无效"})
		return
	}

	// 验证邮箱归属
	_, err = h.store.GetMailbox(c.Request.Context(), mailboxID, account.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "邮箱不存在或已过期"})
		return
	}

	emailID, err := parseUUID(c.Param("email_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮件 ID 无效"})
		return
	}

	if err := h.store.DeleteEmail(c.Request.Context(), emailID, mailboxID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "邮件不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "邮件已删除"})
}
