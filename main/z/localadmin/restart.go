package localadmin

import (
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
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
