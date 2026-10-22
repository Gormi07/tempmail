package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"tempmail/middleware"
	"tempmail/model"
	"tempmail/store"

	"github.com/gin-gonic/gin"
)

type MailboxHandler struct {
	store *store.Store
}

func NewMailboxHandler(s *store.Store) *MailboxHandler {
	return &MailboxHandler{store: s}
}

// POST /api/mailboxes - 创建临时邮箱
// 请求体字段均为可选：
//
//	address — 本地部分（@ 前），为空则随机生成
//	domain  — 指定域名（须是已激活域名），为空则随机选取
//	mode    — single | multi，未指定时随机生成单域名或多级域名邮箱
//	subdomain — multi 模式下自定义子域名前缀，如 a.b.c
func (h *MailboxHandler) Create(c *gin.Context) {
	account := middleware.GetAccount(c)

	var req model.CreateMailboxReq
	c.ShouldBindJSON(&req)

	address := strings.TrimSpace(req.Address)
	if address == "" {
		address = store.GenerateRandomAddress()
	}
	address = strings.ToLower(address)
	customSubdomain := strings.TrimSpace(req.Subdomain)

	// 读取 TTL 设置
	ttlMinutes := 30
	if ttlStr, err := h.store.GetSetting(c.Request.Context(), "mailbox_ttl_minutes"); err == nil {
		if n, err := strconv.Atoi(ttlStr); err == nil && n > 0 {
			ttlMinutes = n
		}
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	randomMode := false
	if mode == "" {
		if req.DomainID > 0 || strings.TrimSpace(req.Domain) != "" {
			mode = "single"
		} else {
			randomMode = true
			if store.RandomBool() {
				mode = "single"
			} else {
				mode = "multi"
			}
		}
	}
	if mode != "single" && mode != "multi" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱模式无效，只能是单域名（single）或多级域名（multi）"})
		return
	}

	// 确定域名：指定 or 随机。single 使用单域名能力，multi 使用通配子域能力。
	var domainRecord *model.Domain
	if req.DomainID > 0 {
		found, err := h.store.GetDomainByID(c.Request.Context(), req.DomainID)
		if err != nil || !found.IsActive {
			c.JSON(http.StatusBadRequest, gin.H{"error": "域名不存在或未启用"})
			return
		}
		if req.Mode == "" {
			mode = "single"
			if !found.SupportsSingle && found.SupportsWildcard {
				mode = "multi"
			}
		}
		if mode == "single" && !found.SupportsSingle {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该域名不支持单域名邮箱，请换一个域名"})
			return
		}
		if mode == "multi" && !found.SupportsWildcard {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该域名不支持多级子域名邮箱，请换一个域名"})
			return
		}
		domainRecord = found
	} else if d := strings.TrimSpace(strings.ToLower(req.Domain)); d != "" {
		found, err := h.store.GetDomainByName(c.Request.Context(), d)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "域名不存在或未启用：" + d})
			return
		}
		if req.Mode == "" {
			mode = "single"
			if !found.SupportsSingle && found.SupportsWildcard {
				mode = "multi"
			}
		}
		if mode == "single" && !found.SupportsSingle {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该域名不支持单域名邮箱，请换一个域名"})
			return
		}
		if mode == "multi" && !found.SupportsWildcard {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该域名不支持多级子域名邮箱，请换一个域名"})
			return
		}
		domainRecord = found
	} else {
		capability := store.DomainCapabilitySingle
		if mode == "multi" {
			capability = store.DomainCapabilityWildcard
		}
		found, err := h.store.GetRandomActiveDomainByCapability(c.Request.Context(), capability)
		if err != nil && randomMode {
			if mode == "multi" {
				found, err = h.store.GetRandomActiveDomainByCapability(c.Request.Context(), store.DomainCapabilitySingle)
				if err == nil {
					mode = "single"
					capability = store.DomainCapabilitySingle
				}
			} else {
				found, err = h.store.GetRandomActiveDomainByCapability(c.Request.Context(), store.DomainCapabilityWildcard)
				if err == nil {
					mode = "multi"
					capability = store.DomainCapabilityWildcard
				}
			}
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": noActiveDomainMsg(capability)})
			return
		}
		domainRecord = found
	}

	domainName := domainRecord.BaseDomain
	if mode == "multi" {
		if customSubdomain != "" {
			customDomain, err := store.JoinCustomSubdomain(customSubdomain, domainRecord.BaseDomain)
			if err != nil {
				respondError(c, http.StatusBadRequest, "自定义子域名格式不正确，只能包含字母、数字和连字符", err)
				return
			}
			domainName = customDomain
		} else {
			domainName = store.GenerateMultiLevelDomain(domainRecord.BaseDomain)
		}
	}
	fullAddress := fmt.Sprintf("%s@%s", address, domainName)

	mailbox, err := h.store.CreateMailbox(c.Request.Context(), account.ID, address, domainRecord.ID, fullAddress, ttlMinutes)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "该邮箱名已被占用，请换一个"})
			return
		}
		respondError(c, http.StatusInternalServerError, "创建邮箱失败，请稍后再试", err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"mailbox": mailbox})
}

// GET /api/mailboxes - 列出当前账号的邮箱
func (h *MailboxHandler) List(c *gin.Context) {
	account := middleware.GetAccount(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	mailboxes, total, err := h.store.ListMailboxes(c.Request.Context(), account.ID, page, size)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取邮箱列表失败，请稍后再试", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  mailboxes,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// DELETE /api/mailboxes/:id - 删除邮箱
func (h *MailboxHandler) Delete(c *gin.Context) {
	account := middleware.GetAccount(c)
	id, err := parseUUID(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "邮箱 ID 无效"})
		return
	}

	if err := h.store.DeleteMailbox(c.Request.Context(), id, account.ID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "邮箱不存在或已过期"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "邮箱已删除"})
}

func noActiveDomainMsg(capability string) string {
	if capability == store.DomainCapabilityWildcard {
		return "暂无可用的多级子域名，请稍后再试"
	}
	return "暂无可用的域名，请稍后再试"
}
