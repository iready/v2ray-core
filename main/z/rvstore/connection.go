package rvstore

import (
	"fmt"

	"github.com/v2fly/v2ray-core/v5/main/z/wire"
)

// AddressPreset Rocket WebSocket 地址预设。
type AddressPreset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// TLSConfig mTLS 配置（仅 wss 生效）。
type TLSConfig struct {
	Mode       string `json:"mode"`
	CAFile     string `json:"ca_file,omitempty"`
	CertFile   string `json:"cert_file,omitempty"`
	KeyFile    string `json:"key_file,omitempty"`
	P12File    string `json:"p12_file,omitempty"`
	P12Pass    string `json:"p12_pass,omitempty"`
	ServerName string `json:"server_name,omitempty"`
	Insecure   bool   `json:"insecure,omitempty"`
}

// Connection agent 连接配置（agent.json connection 段）。
type Connection struct {
	Presets []AddressPreset `json:"presets"`
	Active  string          `json:"active"`
	Token   string          `json:"token"`
	Sign    string          `json:"sign"`
	TLS     TLSConfig       `json:"tls"`
}

// 构建时由 build.sh -ldflags -X 注入；空则使用开发默认。
var (
	buildDefaultRocketURL  string
	buildDefaultRocketName string
	buildDefaultTLSMode    string
)

func DefaultConnection() Connection {
	presetName := "local"
	url := "ws://127.0.0.1:49915/rpc"
	tlsMode := "off"

	if buildDefaultRocketURL != "" {
		url = buildDefaultRocketURL
		presetName = buildDefaultRocketName
		if presetName == "" {
			presetName = "prod"
		}
	}
	if buildDefaultTLSMode != "" {
		tlsMode = buildDefaultTLSMode
	} else if wire.IsWSS(url) {
		tlsMode = "embed"
	}

	return Connection{
		Presets: []AddressPreset{
			{Name: presetName, URL: url},
		},
		Active: presetName,
		TLS:    TLSConfig{Mode: tlsMode},
	}
}

func (c *Connection) Validate() error {
	if c.Active != "" {
		if _, ok := c.PresetByName(c.Active); !ok && c.ResolvedAddr() == "" {
			return fmt.Errorf("unknown active preset %q", c.Active)
		}
	}
	mode := c.TLS.Mode
	if mode == "" {
		c.TLS.Mode = "off"
		mode = "off"
	}
	switch mode {
	case "off", "embed", "file":
	default:
		return fmt.Errorf("invalid tls mode %q", mode)
	}
	if mode == "file" {
		hasPEM := c.TLS.CertFile != "" && c.TLS.KeyFile != ""
		hasP12 := c.TLS.P12File != "" && c.TLS.P12Pass != ""
		if c.TLS.CAFile == "" || (!hasPEM && !hasP12) {
			return fmt.Errorf("tls file mode requires ca_file and (cert_file+key_file or p12_file+p12_pass)")
		}
	}
	return nil
}

func (c Connection) PresetByName(name string) (AddressPreset, bool) {
	for _, p := range c.Presets {
		if p.Name == name {
			return p, true
		}
	}
	return AddressPreset{}, false
}

func (c Connection) ResolvedAddr() string {
	if c.Active != "" {
		if p, ok := c.PresetByName(c.Active); ok && p.URL != "" {
			return p.URL
		}
	}
	if len(c.Presets) > 0 && c.Presets[0].URL != "" {
		return c.Presets[0].URL
	}
	return ""
}

func (c Connection) IsComplete() bool {
	return c.ResolvedAddr() != "" && c.Token != ""
}

func (c Connection) WireTLS() (*wire.TLSConfig, error) {
	opts := wire.TLSOptions{
		Mode:       c.TLS.Mode,
		CAFile:     c.TLS.CAFile,
		CertFile:   c.TLS.CertFile,
		KeyFile:    c.TLS.KeyFile,
		P12File:    c.TLS.P12File,
		P12Pass:    c.TLS.P12Pass,
		ServerName: c.TLS.ServerName,
		Insecure:   c.TLS.Insecure,
	}
	return opts.ForURL(c.ResolvedAddr())
}

func (c *Connection) Masked() Connection {
	out := *c
	if len(out.Token) > 4 {
		out.Token = out.Token[:4] + "***"
	} else if out.Token != "" {
		out.Token = "***"
	}
	if out.TLS.P12Pass != "" {
		out.TLS.P12Pass = "***"
	}
	return out
}

func normalizeConnection(c Connection) Connection {
	if len(c.Presets) == 0 {
		def := DefaultConnection()
		c.Presets = def.Presets
		if c.Active == "" {
			c.Active = def.Active
		}
	}
	if c.TLS.Mode == "" {
		c.TLS.Mode = "off"
	}
	return c
}
