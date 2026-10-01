package localadmin

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/v2fly/v2ray-core/v5/main/z/connlog"
)

func trafficLimit(c *gin.Context) int {
	n, err := strconv.Atoi(c.DefaultQuery("limit", "500"))
	if err != nil || n <= 0 {
		return 500
	}
	if n > 5000 {
		return 5000
	}
	return n
}

func (h *Handler) GetTraffic(c *gin.Context) {
	c.JSON(http.StatusOK, connlog.Snapshot(trafficLimit(c)))
}

func (h *Handler) PostTrafficStart(c *gin.Context) {
	connlog.Start()
	c.JSON(http.StatusOK, connlog.Snapshot(trafficLimit(c)))
}

func (h *Handler) PostTrafficStop(c *gin.Context) {
	connlog.Stop()
	c.JSON(http.StatusOK, connlog.Snapshot(trafficLimit(c)))
}
