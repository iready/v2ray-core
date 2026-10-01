package mitmctl

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/pkcs12"
)

const (
	caBundleFile = "mitmproxy-ca.pem"
	caCertPEM    = "mitmproxy-ca-cert.pem"
	caCertCER    = "mitmproxy-ca-cert.cer"
)

// CABundlePath 私钥+证书 PEM（go-mitmproxy 加载用）。
func CABundlePath() string {
	return filepath.Join(CertDir(), caBundleFile)
}

// ExportCABase64 kind=bundle|cert；bundle 含私钥，cert 仅公钥证书。
func (m *Manager) ExportCABase64(kind string) (string, error) {
	raw, err := m.exportCABytes(kind)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func (m *Manager) exportCABytes(kind string) ([]byte, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" || kind == "cert" {
		pemBytes, err := m.CACertPEM()
		if err != nil {
			return nil, err
		}
		return pemBytes, nil
	}
	if kind != "bundle" {
		return nil, fmt.Errorf("kind 须为 bundle 或 cert")
	}
	data, err := os.ReadFile(CABundlePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("CA 尚未生成，请先启用抓包")
		}
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("CA bundle 为空")
	}
	return data, nil
}

// ImportCAContent 接受 PEM bundle 或 SSL「导出 Base64」的 P12+密码。
// go-mitmproxy 根 CA 仅支持 RSA；EC 会明确拒绝。
func (m *Manager) ImportCAContent(content, password string) error {
	pemBytes, err := decodeCAInput(content, password)
	if err != nil {
		return err
	}
	keyBlock, certBlock, err := splitCABundle(pemBytes)
	if err != nil {
		return err
	}
	keyBlock, certBlock, err = sanitizeMITMCABlocks(keyBlock, certBlock)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(CertDir(), 0755); err != nil {
		return err
	}

	m.mu.Lock()
	wasRunning := m.running
	profile := m.profile.Clone()
	if profile.Addr == "" {
		profile = normalizeProfile(profile)
	}
	_ = m.stopLocked()
	m.mu.Unlock()

	bundle := append(pem.EncodeToMemory(keyBlock), pem.EncodeToMemory(certBlock)...)
	if err := os.WriteFile(CABundlePath(), bundle, 0600); err != nil {
		return err
	}
	if err := writeCertOnlyFiles(certBlock.Bytes); err != nil {
		return err
	}

	if wasRunning || profile.Use {
		profile.Use = true
		return m.Apply(profile)
	}
	return nil
}

// ResetCA 删除损坏/不兼容的 CA，下次启用会重新生成 RSA CA。
func (m *Manager) ResetCA() error {
	m.mu.Lock()
	wasUse := m.profile.Use || m.running
	profile := m.profile.Clone()
	_ = m.stopLocked()
	m.mu.Unlock()

	for _, name := range []string{caBundleFile, caCertPEM, caCertCER} {
		_ = os.Remove(filepath.Join(CertDir(), name))
	}
	if wasUse {
		profile.Use = true
		return m.Apply(profile)
	}
	return nil
}

func sanitizeMITMCABlocks(keyBlock, certBlock *pem.Block) (*pem.Block, *pem.Block, error) {
	// pkcs12.ToPEM 会带 localKeyId 等 Headers，清掉以免部分解析器失败
	keyClean := &pem.Block{Type: keyBlock.Type, Bytes: keyBlock.Bytes}
	certClean := &pem.Block{Type: "CERTIFICATE", Bytes: certBlock.Bytes}

	key, err := parsePrivateKeyBlock(keyClean)
	if err != nil {
		return nil, nil, fmt.Errorf("私钥无法解析: %w", err)
	}
	switch key.(type) {
	case *rsa.PrivateKey:
		// ok — go-mitmproxy 只认 RSA
	case *ecdsa.PrivateKey:
		return nil, nil, fmt.Errorf("抓包引擎只支持 RSA CA，当前 P12 是 EC 私钥。请换 RSA 证书导出，或点「重置为内置 CA」")
	default:
		return nil, nil, fmt.Errorf("不支持的私钥类型 %T（需要 RSA）", key)
	}

	cert, err := x509.ParseCertificate(certClean.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("证书无法解析: %w", err)
	}
	if err := validateMITMRootCA(cert); err != nil {
		return nil, nil, err
	}
	// 统一写成 PKCS#8 PRIVATE KEY，与 go-mitmproxy 生成格式一致
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return &pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}, certClean, nil
}

// validateMITMRootCA：下载给 iOS 装的必须是合格根 CA；否则「下载」按钮也会发出废证。
func validateMITMRootCA(cert *x509.Certificate) error {
	if cert == nil {
		return fmt.Errorf("证书为空")
	}
	if !cert.IsCA {
		return fmt.Errorf("不是根 CA（缺少 BasicConstraints CA:TRUE）。业务/叶证书不能当抓包 CA，请用「重置为内置 CA」或导出真正的根证书")
	}
	if cert.NotAfter.Before(cert.NotBefore) || !cert.NotAfter.After(cert.NotBefore) {
		return fmt.Errorf("证书有效期无效（NotBefore=%s NotAfter=%s）", cert.NotBefore.UTC(), cert.NotAfter.UTC())
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return fmt.Errorf("证书不在有效期内（%s ~ %s）", cert.NotBefore.UTC().Format(time.RFC3339), cert.NotAfter.UTC().Format(time.RFC3339))
	}
	// 公元 1 年等异常日期：iOS 不会进「证书信任设置」
	if cert.NotBefore.Year() < 2000 || cert.NotAfter.Year() < 2000 {
		return fmt.Errorf("证书日期异常（NotBefore=%s），iOS 无法作为受信任根证书", cert.NotBefore.UTC())
	}
	if cert.Subject.CommonName == "" && len(cert.Subject.Organization) == 0 {
		return fmt.Errorf("证书主题为空（无 CN/O），iOS 信任列表不会展示")
	}
	return nil
}

func parsePrivateKeyBlock(block *pem.Block) (any, error) {
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("无法识别私钥 PEM 类型 %q", block.Type)
}

func decodeCAInput(content, password string) ([]byte, error) {
	s := strings.TrimSpace(content)
	if s == "" {
		return nil, fmt.Errorf("内容为空")
	}
	s = strings.Trim(s, "\"'`")
	s = strings.ReplaceAll(s, "\r", "")
	if i := strings.Index(strings.ToLower(s), "base64,"); i >= 0 {
		s = s[i+7:]
	}
	if looksLikePEM(s) {
		return []byte(s), nil
	}

	compact := stripBase64Noise(s)
	raw, err := decodeBase64Flex(compact)
	if err != nil {
		return nil, fmt.Errorf("既非 PEM 也非合法 Base64: %w", err)
	}
	if looksLikePEMBytes(raw) {
		return raw, nil
	}
	if looksLikeDER(raw) {
		if strings.TrimSpace(password) == "" {
			return nil, fmt.Errorf("检测到 P12/二进制内容，请输入导出时的密码")
		}
		return pkcs12ToPEMBundle(raw, password)
	}
	preview := raw
	if len(preview) > 24 {
		preview = preview[:24]
	}
	return nil, fmt.Errorf("无法识别内容（前缀 hex=%x）", preview)
}

func pkcs12ToPEMBundle(pfx []byte, password string) ([]byte, error) {
	blocks, err := pkcs12.ToPEM(pfx, password)
	if err != nil {
		return nil, fmt.Errorf("P12 解密失败（请确认密码）: %w", err)
	}
	var keyPEM, certPEM []byte
	for _, block := range blocks {
		if block == nil {
			continue
		}
		// 去掉 bag attributes，避免写入怪异 PEM
		clean := &pem.Block{Type: block.Type, Bytes: block.Bytes}
		encoded := pem.EncodeToMemory(clean)
		if len(encoded) == 0 {
			continue
		}
		t := strings.ToUpper(block.Type)
		switch {
		case strings.Contains(t, "PRIVATE KEY") && len(keyPEM) == 0:
			keyPEM = encoded
		case strings.Contains(t, "CERTIFICATE") && len(certPEM) == 0:
			certPEM = encoded
		}
	}
	if len(keyPEM) == 0 || len(certPEM) == 0 {
		return nil, fmt.Errorf("P12 中缺少私钥或证书")
	}
	return append(keyPEM, certPEM...), nil
}

func stripBase64Noise(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n', r == '\t', r == ' ', r == '\r':
			return -1
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, s)
}

func decodeBase64Flex(s string) ([]byte, error) {
	try := []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
	}
	var last error
	for _, fn := range try {
		raw, err := fn(s)
		if err == nil {
			return raw, nil
		}
		last = err
	}
	padded := s
	switch len(padded) % 4 {
	case 2:
		padded += "=="
	case 3:
		padded += "="
	}
	if padded != s {
		if raw, err := base64.StdEncoding.DecodeString(padded); err == nil {
			return raw, nil
		}
		if raw, err := base64.URLEncoding.DecodeString(padded); err == nil {
			return raw, nil
		}
	}
	return nil, last
}

func looksLikePEM(s string) bool {
	return strings.Contains(s, "-----BEGIN")
}

func looksLikePEMBytes(b []byte) bool {
	return bytes.Contains(b, []byte("-----BEGIN"))
}

func looksLikeDER(b []byte) bool {
	return len(b) > 16 && b[0] == 0x30
}

func splitCABundle(pemBytes []byte) (keyBlock, certBlock *pem.Block, err error) {
	rest := pemBytes
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		t := block.Type
		switch {
		case strings.Contains(t, "PRIVATE KEY"):
			if keyBlock == nil {
				keyBlock = block
			}
		case t == "CERTIFICATE":
			if certBlock == nil {
				certBlock = block
			}
		}
	}
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("缺少 PRIVATE KEY：请导入 P12 Base64（带密码）或 PEM Bundle")
	}
	if certBlock == nil {
		return nil, nil, fmt.Errorf("缺少 CERTIFICATE")
	}
	return keyBlock, certBlock, nil
}
