package localadmin

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *Handler) PostRestart(c *gin.Context) {
	if h.hooks.RequestShutdown == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "shutdown hook unavailable"})
		return
	}
	if err := spawnRelaunch(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "正在清理并重启，页面稍后自动恢复"})
	h.requestExit("admin_restart")
}

func (h *Handler) PostStop(c *gin.Context) {
	if h.hooks.RequestShutdown == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "shutdown hook unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "正在清理并退出"})
	h.requestExit("admin_stop")
}

func (h *Handler) requestExit(reason string) {
	log.Printf("phase=exit_request reason=%s", reason)
	go func() { time.Sleep(200 * time.Millisecond); h.hooks.RequestShutdown(reason) }()
}
