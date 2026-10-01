package localadmin

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

func localRequestGuard(h *Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Next()
			return
		}
		host, _, err := net.SplitHostPort(c.Request.Host)
		if err != nil {
			host = strings.Trim(c.Request.Host, "[]")
		}
		ip := net.ParseIP(host)
		if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "本地管理接口仅允许 loopback Host"})
			return
		}
		if origin := c.GetHeader("Origin"); origin != "" {
			parsed, parseErr := url.Parse(origin)
			if parseErr != nil || parsed.Host != c.Request.Host || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "跨源请求已拒绝"})
				return
			}
		}
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			supplied := c.GetHeader("X-Rocket-CSRF")
			if h.csrfErr != nil || supplied == "" || subtle.ConstantTimeCompare([]byte(supplied), []byte(h.csrfToken)) != 1 {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "本地会话已失效，请刷新页面后重试"})
				return
			}
		}
		c.Next()
	}
}
