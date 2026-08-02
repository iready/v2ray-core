package mitmctl

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lqqyt2423/go-mitmproxy/proxy"
	"github.com/lqqyt2423/go-mitmproxy/web"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

// Status MITM 运行态。
type Status struct {
	Running     bool   `json:"running"`
	Addr        string `json:"addr,omitempty"`
	WebAddr     string `json:"web_addr,omitempty"` // 可选遗留独立页；空表示仅用 localadmin 内嵌
	Upstream    string `json:"upstream,omitempty"`
	CACertPath  string `json:"ca_cert_path,omitempty"`
	Error       string `json:"error,omitempty"`
	SslInsecure bool   `json:"ssl_insecure,omitempty"`
	FlowCount   int    `json:"flow_count"`
}

type Manager struct {
	mu      sync.Mutex
	proxy   *proxy.Proxy
	profile rvstore.MitmProfile
	running bool
	lastErr string
	flows   *FlowStore
}

var defaultMgr = &Manager{flows: NewFlowStore(defaultFlowCap)}

func Default() *Manager { return defaultMgr }

func CertDir() string {
	return filepath.Join(rvstore.ConfigDir(), "mitm")
}

func CACertPath() string {
	return filepath.Join(CertDir(), "mitmproxy-ca-cert.pem")
}

func (m *Manager) Flows() *FlowStore {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.flows == nil {
		m.flows = NewFlowStore(defaultFlowCap)
	}
	return m.flows
}

func (m *Manager) Profile() rvstore.MitmProfile {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.profile.Addr == "" && !m.running {
		return rvstore.DefaultMitmProfile()
	}
	return m.profile.Clone()
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	flowCount := 0
	if m.flows != nil {
		flowCount = m.flows.Len()
	}
	return Status{
		Running:     m.running,
		Addr:        m.profile.EffectiveAddr(),
		WebAddr:     m.profile.EffectiveWebAddr(),
		Upstream:    m.profile.Upstream,
		SslInsecure: m.profile.SslInsecure,
		CACertPath:  CACertPath(),
		Error:       m.lastErr,
		FlowCount:   flowCount,
	}
}

// Apply 按配置启停；Use=false 时停止。
func (m *Manager) Apply(profile rvstore.MitmProfile) error {
	profile = normalizeProfile(profile)

	m.mu.Lock()
	if m.flows == nil {
		m.flows = NewFlowStore(defaultFlowCap)
	}
	flows := m.flows
	if err := m.stopLocked(); err != nil {
		m.lastErr = err.Error()
		m.mu.Unlock()
		return err
	}
	m.profile = profile
	m.lastErr = ""
	if !profile.Use {
		m.mu.Unlock()
		return nil
	}
	p, err := newProxy(profile, flows)
	if err != nil {
		m.lastErr = err.Error()
		m.mu.Unlock()
		return err
	}
	m.proxy = p
	m.running = true
	m.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		err := p.Start()
		if err != nil {
			m.mu.Lock()
			if m.proxy == p {
				m.running = false
				m.lastErr = err.Error()
				m.proxy = nil
			}
			m.mu.Unlock()
		}
		errCh <- err
	}()
	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
		_ = syncDeviceCAFiles()
		return nil
	case <-time.After(120 * time.Millisecond):
		// go-mitmproxy 生成的 .cer 常是 PEM；统一洗成 DER，下载按钮才适合 iOS
		_ = syncDeviceCAFiles()
		return nil
	}
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.stopLocked()
	if err != nil {
		m.lastErr = err.Error()
	} else {
		m.lastErr = ""
	}
	return err
}

func (m *Manager) Enable() error {
	m.mu.Lock()
	p := m.profile
	if p.Addr == "" {
		p = rvstore.DefaultMitmProfile()
	}
	m.mu.Unlock()
	p.Use = true
	return m.Apply(p)
}

func (m *Manager) Disable() error {
	m.mu.Lock()
	p := m.profile
	if p.Addr == "" {
		p = rvstore.DefaultMitmProfile()
	}
	m.mu.Unlock()
	p.Use = false
	return m.Apply(p)
}

// CACertPEM 返回干净的 CA 证书 PEM（无 PKCS12 bag 头，便于 macOS/iOS 解析）。
func (m *Manager) CACertPEM() ([]byte, error) {
	cert, err := m.loadCACertificate()
	if err != nil {
		return nil, err
	}
	return encodeCertPEM(cert), nil
}

// CACertDER 返回 DER（iOS 隔空投送 / 安装请用 .cer）。
func (m *Manager) CACertDER() ([]byte, error) {
	cert, err := m.loadCACertificate()
	if err != nil {
		return nil, err
	}
	if err := validateMITMRootCA(cert); err != nil {
		return nil, fmt.Errorf("当前 CA 不适合安装到设备: %w（请点「重置为内置 CA」后再下载）", err)
	}
	// 顺手把脏文件洗掉，避免下次又下到坏文件
	_ = writeCertOnlyFiles(cert.Raw)
	return append([]byte(nil), cert.Raw...), nil
}

func (m *Manager) loadCACertificate() (*x509.Certificate, error) {
	for _, path := range []string{CACertPath(), filepath.Join(CertDir(), caCertCER), CABundlePath()} {
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			continue
		}
		if cert, err := parseCertificateBytes(data); err == nil {
			return cert, nil
		}
	}
	m.mu.Lock()
	p := m.proxy
	m.mu.Unlock()
	if p == nil {
		return nil, fmt.Errorf("CA 尚未生成，请先启用抓包")
	}
	root := p.GetCertificate()
	return &root, nil
}

func parseCertificateBytes(data []byte) (*x509.Certificate, error) {
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			return x509.ParseCertificate(block.Bytes)
		}
	}
	// 裸 DER
	if len(data) > 16 && data[0] == 0x30 {
		return x509.ParseCertificate(data)
	}
	return nil, fmt.Errorf("未找到证书")
}

// writeCertOnlyFiles 写干净 PEM + DER .cer（iOS 认 DER）。
func writeCertOnlyFiles(certDER []byte) error {
	if _, err := x509.ParseCertificate(certDER); err != nil {
		return fmt.Errorf("证书 DER 无效: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err := os.WriteFile(CACertPath(), pemBytes, 0644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(CertDir(), caCertCER), certDER, 0644)
}

// syncDeviceCAFiles 把当前磁盘 CA 洗成无 bag 头的 PEM + 真 DER .cer。
func syncDeviceCAFiles() error {
	for _, path := range []string{CACertPath(), filepath.Join(CertDir(), caCertCER), CABundlePath()} {
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			continue
		}
		cert, err := parseCertificateBytes(data)
		if err != nil {
			continue
		}
		return writeCertOnlyFiles(cert.Raw)
	}
	return fmt.Errorf("无可用 CA 文件可同步")
}

func newProxy(profile rvstore.MitmProfile, flows *FlowStore) (*proxy.Proxy, error) {
	caDir := CertDir()
	if err := os.MkdirAll(caDir, 0755); err != nil {
		return nil, err
	}
	opts := &proxy.Options{
		Addr:              profile.EffectiveAddr(),
		StreamLargeBodies: 1024 * 1024 * 5,
		SslInsecure:       profile.SslInsecure,
		CaRootPath:        caDir,
		Upstream:          strings.TrimSpace(profile.Upstream),
	}
	p, err := proxy.NewProxy(opts)
	if err != nil {
		return nil, err
	}
	ignore := profile.EffectiveIgnoreHosts()
	if len(ignore) > 0 {
		p.SetShouldInterceptRule(func(req *http.Request) bool {
			return !matchHost(req.Host, ignore)
		})
	}
	p.AddAddon(&proxy.LogAddon{})
	p.AddAddon(NewMapRemoteAddon(profile.MapRemote))
	p.AddAddon(NewRecorderAddon(flows))
	// 可选：遗留独立 go-mitmproxy 页；默认空，流量看 localadmin
	if webAddr := profile.EffectiveWebAddr(); webAddr != "" {
		p.AddAddon(web.NewWebAddon(webAddr))
	}
	return p, nil
}

func (m *Manager) stopLocked() error {
	if m.proxy == nil {
		m.running = false
		return nil
	}
	p := m.proxy
	m.proxy = nil
	m.running = false
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := p.Shutdown(ctx); err != nil {
		_ = p.Close()
		return err
	}
	return nil
}

func normalizeProfile(p rvstore.MitmProfile) rvstore.MitmProfile {
	d := rvstore.DefaultMitmProfile()
	if strings.TrimSpace(p.Addr) == "" {
		p.Addr = d.Addr
	}
	if p.IgnoreHosts == nil {
		p.IgnoreHosts = append([]string(nil), d.IgnoreHosts...)
	}
	return p
}

func matchHost(host string, patterns []string) bool {
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	h = strings.ToLower(strings.TrimSpace(h))
	for _, raw := range patterns {
		p := strings.ToLower(strings.TrimSpace(raw))
		if p == "" {
			continue
		}
		if h == p || strings.HasSuffix(h, "."+p) {
			return true
		}
	}
	return false
}

func encodeCertPEM(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}
