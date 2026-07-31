package localadmin

import (
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/v2fly/v2ray-core/v5/main/z/mitmctl"
)

func (h *Handler) PostRestart(c *gin.Context) {
	if err := spawnRelaunch(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "正在重启，页面稍后自动恢复"})
	go func() {
		time.Sleep(300 * time.Millisecond)
		os.Exit(0)
	}()
}

func (h *Handler) PostStop(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "正在退出"})
	go func() {
		time.Sleep(200 * time.Millisecond)
		if h.hooks.GetRS != nil {
			if rs := h.hooks.GetRS(); rs != nil {
				rs.StopAllServers()
				rs.DisconnectWire()
			}
		}
		_ = mitmctl.Default().Stop()
		os.Exit(0)
	}()
}
