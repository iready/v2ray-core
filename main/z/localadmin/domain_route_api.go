package localadmin

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/v2fly/v2ray-core/v5/main/z/domainreq"
)

func (h *Handler) GetDomainRouteRequests(c *gin.Context) {
	items, err := domainreq.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (h *Handler) PostDomainRouteRequest(c *gin.Context) {
	var body struct {
		Domains []string `json:"domains"`
		Remark  string   `json:"remark"`
		Text    string   `json:"text"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	domains := body.Domains
	if body.Text != "" {
		domains = append(domains, body.Text)
	}
	item, err := domainreq.Create(domains, body.Remark)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 在线则立刻上报；失败仍保留 pending，稍后 Flush。
	if rs := h.hooks.GetRS(); rs != nil && rs.WireConnected() {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		_, _ = domainreq.FlushPending(ctx, rs)
		items, _ := domainreq.List()
		for _, it := range items {
			if it.ClientReqID == item.ClientReqID {
				item = it
				break
			}
		}
	}
	c.JSON(http.StatusOK, item)
}

func (h *Handler) PostDomainRouteRequestFlush(c *gin.Context) {
	rs := h.hooks.GetRS()
	if rs == nil || !rs.WireConnected() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "wire 未连接"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	n, err := domainreq.FlushPending(ctx, rs)
	if err != nil && n == 0 {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "flushed": n})
		return
	}
	items, _ := domainreq.List()
	c.JSON(http.StatusOK, gin.H{"flushed": n, "items": items, "error": errString(err)})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
