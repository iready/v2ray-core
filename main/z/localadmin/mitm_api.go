package localadmin

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/v2fly/v2ray-core/v5/main/z/mitmctl"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

// MitmProfileResponse 抓包配置 + 运行态。
type MitmProfileResponse struct {
	Use         bool                        `json:"use"`
	Addr        string                      `json:"addr,omitempty"`
	WebAddr     string                      `json:"web_addr,omitempty"`
	Upstream    string                      `json:"upstream,omitempty"`
	SslInsecure bool                        `json:"ssl_insecure,omitempty"`
	IgnoreHosts []string                    `json:"ignore_hosts,omitempty"`
	MediaBypass bool                        `json:"media_bypass,omitempty"`
	MapRemote   []rvstore.MitmMapRemoteRule `json:"map_remote,omitempty"`
	HostCerts   []rvstore.MitmHostCertRule  `json:"host_certs,omitempty"`
	Running     bool                        `json:"running"`
	CACertPath  string                      `json:"ca_cert_path,omitempty"`
	Error       string                      `json:"error,omitempty"`
	Port        string                      `json:"port,omitempty"`
	Connect     []mitmctl.ConnectEndpoint   `json:"connect,omitempty"`
}

func mitmProfileResponse(profile rvstore.MitmProfile, st mitmctl.Status) MitmProfileResponse {
	addr := profile.EffectiveAddr()
	return MitmProfileResponse{
		Use:         profile.Use,
		Addr:        addr,
		WebAddr:     profile.EffectiveWebAddr(),
		Upstream:    profile.Upstream,
		SslInsecure: profile.SslInsecure,
		IgnoreHosts: profile.IgnoreHosts,
		MediaBypass: profile.MediaBypass,
		MapRemote:   profile.MapRemote,
		HostCerts:   redactHostCertKeys(profile.HostCerts),
		Running:     st.Running,
		CACertPath:  st.CACertPath,
		Error:       st.Error,
		Port:        mitmctl.ListenPort(addr),
		Connect:     mitmctl.ListConnectEndpoints(addr),
	}
}

// redactHostCertKeys：HTTP 响应不回传私钥；仅标记 has_key。
func redactHostCertKeys(in []rvstore.MitmHostCertRule) []rvstore.MitmHostCertRule {
	if len(in) == 0 {
		return in
	}
	out := make([]rvstore.MitmHostCertRule, len(in))
	for i, r := range in {
		out[i] = r
		if strings.TrimSpace(r.KeyPEM) != "" {
			out[i].HasKey = true
		}
		out[i].KeyPEM = ""
	}
	return out
}

// mergeHostCertKeys：客户端保存时若 key_pem 为空，保留磁盘上同 id/host 的旧钥。
func mergeHostCertKeys(incoming, existing []rvstore.MitmHostCertRule) []rvstore.MitmHostCertRule {
	if len(incoming) == 0 {
		return incoming
	}
	byID := make(map[string]rvstore.MitmHostCertRule, len(existing))
	byHost := make(map[string]rvstore.MitmHostCertRule, len(existing))
	for _, e := range existing {
		if e.ID != "" {
			byID[e.ID] = e
		}
		h := strings.ToLower(strings.TrimSpace(e.Host))
		if h != "" {
			byHost[h] = e
		}
	}
	out := make([]rvstore.MitmHostCertRule, len(incoming))
	copy(out, incoming)
	for i := range out {
		out[i].HasKey = false // 不落盘
		if strings.TrimSpace(out[i].KeyPEM) != "" {
			continue
		}
		var old rvstore.MitmHostCertRule
		var ok bool
		if out[i].ID != "" {
			old, ok = byID[out[i].ID]
		}
		if !ok {
			old, ok = byHost[strings.ToLower(strings.TrimSpace(out[i].Host))]
		}
		if ok {
			out[i].KeyPEM = old.KeyPEM
			if strings.TrimSpace(out[i].CertPEM) == "" {
				out[i].CertPEM = old.CertPEM
			}
		}
	}
	return out
}

func (h *Handler) GetMitmStatus(c *gin.Context) {
	st := mitmctl.Default().Status()
	c.JSON(http.StatusOK, gin.H{
		"running":      st.Running,
		"addr":         st.Addr,
		"web_addr":     st.WebAddr,
		"upstream":     st.Upstream,
		"ca_cert_path": st.CACertPath,
		"error":        st.Error,
		"ssl_insecure": st.SslInsecure,
		"flow_count":   st.FlowCount,
		"port":         mitmctl.ListenPort(st.Addr),
		"connect":      mitmctl.ListConnectEndpoints(st.Addr),
	})
}

func (h *Handler) GetMitmProfile(c *gin.Context) {
	profile, err := h.store.LoadMitmProfile()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, mitmProfileResponse(profile, mitmctl.Default().Status()))
}

func (h *Handler) PutMitmProfile(c *gin.Context) {
	var incoming rvstore.MitmProfile
	if err := c.ShouldBindJSON(&incoming); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if existing, err := h.store.LoadMitmProfile(); err == nil {
		incoming.HostCerts = mergeHostCertKeys(incoming.HostCerts, existing.HostCerts)
	}
	if err := h.persistMitmProfile(incoming); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	saved, _ := h.store.LoadMitmProfile()
	c.JSON(http.StatusOK, mitmProfileResponse(saved, mitmctl.Default().Status()))
}

func (h *Handler) PostMitmEnable(c *gin.Context) {
	profile, err := h.store.LoadMitmProfile()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	profile.Use = true
	if err := h.persistMitmProfile(profile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, mitmctl.Default().Status())
}

func (h *Handler) PostMitmDisable(c *gin.Context) {
	profile, err := h.store.LoadMitmProfile()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	profile.Use = false
	if err := h.persistMitmProfile(profile); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, mitmctl.Default().Status())
}

func (h *Handler) GetMitmCA(c *gin.Context) {
	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "cer")))
	if format == "pem" {
		pemBytes, err := mitmctl.Default().CACertPEM()
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.Header("Content-Disposition", `attachment; filename="rocket-mitm-ca.pem"`)
		c.Data(http.StatusOK, "application/x-pem-file", pemBytes)
		return
	}
	der, err := mitmctl.Default().CACertDER()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	// iOS 隔空投送 / 设置安装认 DER .cer；带 bag 头的 PEM 会报「无效的描述文件」
	c.Header("Content-Disposition", `attachment; filename="rocket-mitm-ca.cer"`)
	c.Data(http.StatusOK, "application/x-x509-ca-cert", der)
}

func (h *Handler) GetMitmCABase64(c *gin.Context) {
	kind := c.DefaultQuery("kind", "cert")
	content, err := mitmctl.Default().ExportCABase64(kind)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"kind": kind, "content": content})
}

type mitmCAImportReq struct {
	Content  string `json:"content"`
	Password string `json:"password"`
}

func (h *Handler) PostMitmCAImport(c *gin.Context) {
	var req mitmCAImportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := mitmctl.Default().ImportCAContent(req.Content, req.Password); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	profile, _ := h.store.LoadMitmProfile()
	c.JSON(http.StatusOK, mitmProfileResponse(profile, mitmctl.Default().Status()))
}

func (h *Handler) PostMitmCAReset(c *gin.Context) {
	if err := mitmctl.Default().ResetCA(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	profile, _ := h.store.LoadMitmProfile()
	c.JSON(http.StatusOK, mitmProfileResponse(profile, mitmctl.Default().Status()))
}

type mitmHostCertParseReq struct {
	Content  string `json:"content"`
	Password string `json:"password"`
}

func (h *Handler) PostMitmHostCertParse(c *gin.Context) {
	var req mitmHostCertParseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	certPEM, keyPEM, err := mitmctl.ParseHostCertMaterial(req.Content, req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cert_pem": certPEM, "key_pem": keyPEM})
}

func (h *Handler) GetMitmFlows(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"flows": mitmctl.Default().Flows().List()})
}

func (h *Handler) GetMitmFlow(c *gin.Context) {
	id := c.Param("id")
	detail, ok := mitmctl.Default().Flows().Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "flow not found"})
		return
	}
	c.JSON(http.StatusOK, detail)
}

func (h *Handler) DeleteMitmFlows(c *gin.Context) {
	mitmctl.Default().Flows().Clear()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) persistMitmProfile(profile rvstore.MitmProfile) error {
	if err := h.store.SaveMitmProfile(profile); err != nil {
		return err
	}
	return mitmctl.Default().Apply(profile)
}
