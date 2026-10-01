package localadmin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/rocket"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
)

// TunProfileResponse 本地 TUN 配置与远端模板状态。
type TunProfileResponse struct {
	Use                     bool                   `json:"use"`
	TunOwnerKey             string                 `json:"tun_owner_key,omitempty"`
	TunOwnerCandidates      []string               `json:"tun_owner_candidates,omitempty"`
	BindInterface           string                 `json:"bind_interface,omitempty"`
	BindInterfaceCandidates []string               `json:"bind_interface_candidates,omitempty"`
	BindInterfaceEffective  string                 `json:"bind_interface_effective,omitempty"`
	CnDNS                   []string               `json:"cn_dns,omitempty"`
	CnDNSDefault            []string               `json:"cn_dns_default"`
	RemoteDNS               string                 `json:"remote_dns,omitempty"`
	RemoteDNSDefault        string                 `json:"remote_dns_default"`
	FakeDNSDomains          []string               `json:"fakedns_domains,omitempty"`
	BypassRocketServer      *bool                  `json:"bypass_rocket_server,omitempty"`
	BypassLAN               *bool                  `json:"bypass_lan,omitempty"`
	BypassLoopback          *bool                  `json:"bypass_loopback,omitempty"`
	ExtraBypassHosts        []string               `json:"extra_bypass_hosts,omitempty"`
	Services                map[string]interface{} `json:"services,omitempty"`
	ServerHasTemplate       bool                   `json:"server_has_template"`
}

func tunProfileResponse(profile rvstore.TunProfile, serverHasTemplate bool, candidates []string, bindEffective string) TunProfileResponse {
	return TunProfileResponse{
		Use:                     profile.Use,
		TunOwnerKey:             profile.TunOwnerKey,
		TunOwnerCandidates:      candidates,
		BindInterface:           profile.BindInterface,
		BindInterfaceCandidates: tunctl.ListBindInterfaceCandidates(),
		BindInterfaceEffective:  bindEffective,
		CnDNS:                   profile.CnDNS,
		CnDNSDefault:            rvstore.DefaultCNResolvers(),
		RemoteDNS:               profile.RemoteDNS,
		RemoteDNSDefault:        rvstore.DefaultRemoteResolver,
		FakeDNSDomains:          profile.FakeDNSDomains,
		BypassRocketServer:      profile.BypassRocketServer,
		BypassLAN:               profile.BypassLAN,
		BypassLoopback:          profile.BypassLoopback,
		ExtraBypassHosts:        profile.ExtraBypassHosts,
		Services:                profile.Services,
		ServerHasTemplate:       serverHasTemplate,
	}
}

func (h *Handler) tunOwnerCandidates() []string {
	cached, err := h.store.RVStore().LoadConfigCache()
	if err != nil || cached == nil || cached.Config == nil {
		return nil
	}
	return rocket.ListTunOwnerCandidates(cached.Config.Config)
}

func (h *Handler) bindInterfaceEffective() string {
	if iface := tunctl.CurrentBindInterface(); iface != "" {
		return iface
	}
	if h.hooks.GetRS != nil {
		if rs := h.hooks.GetRS(); rs != nil {
			return rs.Status().TunBindInterface
		}
	}
	return ""
}

func (h *Handler) GetTunProfile(c *gin.Context) {
	profile, err := h.store.LoadTunProfile()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tunProfileResponse(profile, h.serverHasTUNTemplate(), h.tunOwnerCandidates(), h.bindInterfaceEffective()))
}

func (h *Handler) PutTunProfile(c *gin.Context) {
	var incoming rvstore.TunProfile
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.persistTunProfile(incoming); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.hooks.OnRebootstrap != nil {
		if err := h.hooks.OnRebootstrap(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, tunProfileResponse(incoming, h.serverHasTUNTemplate(), h.tunOwnerCandidates(), h.bindInterfaceEffective()))
}

func (h *Handler) persistTunProfile(profile rvstore.TunProfile) error {
	if err := h.store.SaveTunProfile(profile); err != nil {
		return err
	}
	tunctl.SetUserDisabled(!profile.Use)
	return nil
}

func (h *Handler) serverHasTUNTemplate() bool {
	var cached *pb.GetConfigRes
	if sc, err := h.store.RVStore().LoadConfigCache(); err == nil && sc != nil {
		cached = sc.Config
	}
	return rocket.ConfigHasTUN(h.hooks.GetRS(), cached)
}
