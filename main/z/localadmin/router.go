package localadmin

import (
	"embed"
	"net/http"
	"strings"

	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

func NewRouter(h *Handler, buildFS embed.FS, indexPage []byte) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	api := r.Group("/api")
	api.GET("/status", h.GetStatus)
	api.GET("/config", h.GetConfig)
	api.GET("/config/server", h.GetServerConfig)
	api.PUT("/config", h.PutConfig)
	api.POST("/reconnect", h.PostReconnect)
	api.GET("/tun/status", h.GetTunStatus)
	api.GET("/tun/profile", h.GetTunProfile)
	api.PUT("/tun/profile", h.PutTunProfile)
	api.GET("/log/profile", h.GetLogProfile)
	api.PUT("/log/profile", h.PutLogProfile)
	api.POST("/tun/enable", h.PostTunEnable)
	api.POST("/tun/disable", h.PostTunDisable)
	api.POST("/tun/install-helper", h.PostTunInstallHelper)
	api.POST("/tun/uninstall-helper", h.PostTunUninstallHelper)
	api.GET("/autostart", h.GetAutostart)
	api.POST("/autostart/install", h.PostAutostartInstall)
	api.POST("/autostart/uninstall", h.PostAutostartUninstall)
	api.POST("/autostart/relaunch", h.PostAutostartRelaunch)
	api.POST("/diagnose", h.PostDiagnose)

	r.Use(gzip.Gzip(gzip.DefaultCompression))
	r.Use(static.Serve("/", static.EmbedFolder(buildFS, WebDistRoot)))
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
	})
	return r
}
