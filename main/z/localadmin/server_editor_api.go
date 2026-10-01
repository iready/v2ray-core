package localadmin

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/v2fly/v2ray-core/v5/main/z/wire"
)

func (h *Handler) editorClient(c *gin.Context) *wire.Client {
	rs := h.hooks.GetRS()
	if rs == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Rocket Agent 未启动"})
		return nil
	}
	client := rs.WireClient()
	if client == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Rocket 服务端连接未就绪"})
		return nil
	}
	return client
}

func (h *Handler) GetMyServers(c *gin.Context) {
	client := h.editorClient(c)
	if client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	result, err := client.ListEditableServers(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) GetMyServer(c *gin.Context) {
	client := h.editorClient(c)
	if client == nil {
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	result, err := client.GetEditableServer(ctx, id)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) PutMyServer(c *gin.Context) {
	client := h.editorClient(c)
	if client == nil {
		return
	}
	var request struct {
		ID      string         `json:"id"`
		Version int64          `json:"version"`
		Config  map[string]any `json:"config"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求 JSON 无效"})
		return
	}
	pathID := strings.TrimSpace(c.Param("id"))
	if pathID == "" || request.Config == nil || (request.ID != "" && request.ID != pathID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Server id 与 config 必填且一致"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	result, err := client.SaveEditableServer(ctx, pathID, request.Version, request.Config)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if strings.Contains(err.Error(), "已被管理员或另一客户端更新") {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}
