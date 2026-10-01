package localadmin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/v2fly/v2ray-core/v5/main/z/rocket"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
)

// Hooks 本地后台与 agent 生命周期集成。
type Hooks struct {
	GetRS         func() *rocket.RS
	GetConfig     func() AgentConfig
	OnSave        func(AgentConfig) error
	OnReconnect   func() error
	OnRebootstrap func() error
}

type Handler struct {
	store *Store
	hooks Hooks
}

func NewHandler(store *Store, hooks Hooks) *Handler {
	return &Handler{store: store, hooks: hooks}
}

func (h *Handler) GetStatus(c *gin.Context) {
	cfg := h.hooks.GetConfig()
	rs := h.hooks.GetRS()
	rt := h.store.LoadRuntime()
	snap := BuildStatus(rs, cfg, rt)
	if cached, err := h.store.RVStore().LoadConfigCache(); err == nil && cached != nil && cached.Config != nil {
		snap.TunInConfig = rocket.ConfigHasTUN(rs, cached.Config)
	}
	if profile, err := h.store.LoadTunProfile(); err == nil {
		snap.TunLocalUse = profile.Use
	}
	c.JSON(http.StatusOK, snap)
}

func (h *Handler) GetConfig(c *gin.Context) {
	cfg := h.hooks.GetConfig()
	c.JSON(http.StatusOK, cfg.Masked())
}

func (h *Handler) PutConfig(c *gin.Context) {
	var incoming AgentConfig
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	current, _ := h.store.Load()
	if incoming.Token == "" || incoming.Token == "***" || (len(incoming.Token) > 4 && incoming.Token[len(incoming.Token)-3:] == "***") {
		incoming.Token = current.Token
	}
	if incoming.TLS.P12Pass == "***" {
		incoming.TLS.P12Pass = current.TLS.P12Pass
	}
	if err := incoming.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.Save(incoming); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.hooks.OnSave != nil {
		if err := h.hooks.OnSave(incoming); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, incoming.Masked())
}

func (h *Handler) PostReconnect(c *gin.Context) {
	if h.hooks.OnReconnect == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent not ready"})
		return
	}
	if err := h.hooks.OnReconnect(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) GetTunStatus(c *gin.Context) {
	c.JSON(http.StatusOK, tunctl.Default().Status())
}

func (h *Handler) PostTunEnable(c *gin.Context) {
	if h.hooks.OnRebootstrap == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent not ready"})
		return
	}
	profile, err := h.store.LoadTunProfile()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	profile.Use = true
	if err := h.persistTunProfile(profile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.hooks.OnRebootstrap(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tunctl.Default().Status())
}

func (h *Handler) PostTunDisable(c *gin.Context) {
	if h.hooks.OnRebootstrap == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "agent not ready"})
		return
	}
	profile, err := h.store.LoadTunProfile()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	profile.Use = false
	if err := h.persistTunProfile(profile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.hooks.OnRebootstrap(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tunctl.Default().Status())
}

func (h *Handler) PostTunInstallHelper(c *gin.Context) {
	if err := tunctl.InstallHelper(tunctl.HelperBinaryPath()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "helper_installed": tunctl.Default().HelperInstalled()})
}

func (h *Handler) PostTunUninstallHelper(c *gin.Context) {
	if err := tunctl.UninstallHelper(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "helper_installed": tunctl.Default().HelperInstalled()})
}
