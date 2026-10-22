package handler

import (
	"log"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// 通用中文错误提示（不向用户暴露内部错误细节）。
const (
	MsgBadRequest = "请求参数不正确"
	MsgInternal   = "服务器内部错误，请稍后再试"
)

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// respondError 返回面向用户的中文错误；内部错误 err 只写入服务端日志。
func respondError(c *gin.Context, status int, msg string, err error) {
	if err != nil {
		log.Printf("[api] %s %s -> %d %s: %v", c.Request.Method, c.Request.URL.Path, status, msg, err)
	}
	c.JSON(status, gin.H{"error": msg})
}
