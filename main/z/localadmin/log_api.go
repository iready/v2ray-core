package localadmin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

// LogProfileResponse 本地日志覆盖与服务端模板状态。
type LogProfileResponse struct {
	Use            bool    `json:"use"`
	Loglevel       *string `json:"loglevel,omitempty"`
	Access         *string `json:"access,omitempty"`
	Error          *string `json:"error,omitempty"`
	ServerLoglevel string  `json:"server_loglevel,omitempty"`
}

func logProfileResponse(profile rvstore.LogProfile, serverLoglevel string) LogProfileResponse {
	return LogProfileResponse{
		Use:            profile.Use,
		Loglevel:       profile.Loglevel,
		Access:         profile.Access,
		Error:          profile.Error,
		ServerLoglevel: serverLoglevel,
	}
}

func (h *Handler) GetLogProfile(c *gin.Context) {
	profile, err := h.store.LoadLogProfile()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, logProfileResponse(profile, h.serverLoglevel()))
}

func (h *Handler) PutLogProfile(c *gin.Context) {
	var incoming rvstore.LogProfile
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.SaveLogProfile(incoming); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.hooks.OnRebootstrap != nil {
		if err := h.hooks.OnRebootstrap(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, logProfileResponse(incoming, h.serverLoglevel()))
}

func (h *Handler) serverLoglevel() string {
	var cached *pb.GetConfigRes
	if sc, err := h.store.RVStore().LoadConfigCache(); err == nil && sc != nil {
		cached = sc.Config
	}
	return serverLoglevelFromConfig(cached)
}

func serverLoglevelFromConfig(cfg *pb.GetConfigRes) string {
	if cfg == nil {
		return ""
	}
	for _, item := range cfg.Config {
		if item == nil || item.Config == "" {
			continue
		}
		if lvl := rvstore.ParseLoglevelFromV2JSON(item.Config); lvl != "" {
			return lvl
		}
	}
	return ""
}
