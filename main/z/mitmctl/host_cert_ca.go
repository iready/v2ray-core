package mitmctl

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"strings"

	"github.com/lqqyt2423/go-mitmproxy/cert"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

// hostCertCA：命中 HostCerts 规则时返回指定叶子证；否则委托全局 SelfSignCA。
type hostCertCA struct {
	base  cert.CA
	exact map[string]*tls.Certificate // lowercase host → cert
	// suffix rules: pattern without leading dot, longer first
	suffix []hostCertSuffix
}

type hostCertSuffix struct {
	suffix string
	cert   *tls.Certificate
}

func NewHostCertCA(base cert.CA, rules []rvstore.MitmHostCertRule) (cert.CA, error) {
	if base == nil {
		return nil, fmt.Errorf("base CA 为空")
	}
	h := &hostCertCA{
		base:  base,
		exact: map[string]*tls.Certificate{},
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		host := normalizeHostPattern(rule.Host)
		if host == "" {
			continue
		}
		tlsCert, err := loadHostTLSCertificate(rule.CertPEM, rule.KeyPEM)
		if err != nil {
			return nil, fmt.Errorf("域名证书 %s: %w", rule.Host, err)
		}
		if strings.HasPrefix(host, "*.") {
			suf := strings.TrimPrefix(host, "*.")
			h.suffix = append(h.suffix, hostCertSuffix{suffix: suf, cert: tlsCert})
			continue
		}
		h.exact[host] = tlsCert
		// 同时允许后缀匹配：配置 example.com 时 a.example.com 也命中
		h.suffix = append(h.suffix, hostCertSuffix{suffix: host, cert: tlsCert})
	}
	return h, nil
}

func (c *hostCertCA) GetRootCA() *x509.Certificate {
	return c.base.GetRootCA()
}

func (c *hostCertCA) GetCert(commonName string) (*tls.Certificate, error) {
	cn := strings.ToLower(strings.TrimSpace(commonName))
	if cn != "" {
		if t := c.exact[cn]; t != nil {
			return t, nil
		}
		best := (*tls.Certificate)(nil)
		bestLen := -1
		for _, s := range c.suffix {
			if cn == s.suffix || strings.HasSuffix(cn, "."+s.suffix) {
				if len(s.suffix) > bestLen {
					best = s.cert
					bestLen = len(s.suffix)
				}
			}
		}
		if best != nil {
			return best, nil
		}
	}
	return c.base.GetCert(commonName)
}

func normalizeHostPattern(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimPrefix(h, "https://")
	h = strings.TrimPrefix(h, "http://")
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	return h
}

func loadHostTLSCertificate(certPEM, keyPEM string) (*tls.Certificate, error) {
	certPEM = strings.TrimSpace(certPEM)
	keyPEM = strings.TrimSpace(keyPEM)
	if certPEM == "" || keyPEM == "" {
		return nil, fmt.Errorf("缺少证书或私钥")
	}
	tlsCert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return nil, fmt.Errorf("证书/私钥无法配对: %w", err)
	}
	if len(tlsCert.Certificate) == 0 {
		return nil, fmt.Errorf("证书为空")
	}
	leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("叶子证书无法解析: %w", err)
	}
	tlsCert.Leaf = leaf
	return &tlsCert, nil
}

// ParseHostCertMaterial 从 P12 Base64+密码或 PEM bundle 解析叶子证+私钥（不要求 CA:TRUE）。
func ParseHostCertMaterial(content, password string) (certPEM, keyPEM string, err error) {
	raw, err := decodeCAInput(content, password)
	if err != nil {
		return "", "", err
	}
	keyBlock, certBlock, err := splitCABundle(raw)
	if err != nil {
		return "", "", err
	}
	// 清 bag headers；允许 RSA/EC（叶子证给客户端看，不经 SelfSignCA 加载私钥）
	keyClean := &pem.Block{Type: keyBlock.Type, Bytes: keyBlock.Bytes}
	certClean := &pem.Block{Type: "CERTIFICATE", Bytes: certBlock.Bytes}
	key, err := parsePrivateKeyBlock(keyClean)
	if err != nil {
		return "", "", fmt.Errorf("私钥无法解析: %w", err)
	}
	leaf, err := x509.ParseCertificate(certClean.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("证书无法解析: %w", err)
	}
	if err := publicKeysMatch(leaf.PublicKey, key); err != nil {
		return "", "", err
	}
	// 统一 PKCS#8
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}
	keyOut := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	// 保留链：split 只取第一张；若 raw 里有多 CERTIFICATE，拼回去
	var certOut []byte
	rest := raw
	for {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		if b.Type == "CERTIFICATE" {
			certOut = append(certOut, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: b.Bytes})...)
		}
	}
	if len(certOut) == 0 {
		certOut = pem.EncodeToMemory(certClean)
	}
	return string(certOut), string(keyOut), nil
}

func publicKeysMatch(pub any, priv any) error {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		p, ok := pub.(*rsa.PublicKey)
		if !ok || k.N.Cmp(p.N) != 0 || k.E != p.E {
			return fmt.Errorf("私钥与证书公钥不匹配（RSA）")
		}
	case *ecdsa.PrivateKey:
		p, ok := pub.(*ecdsa.PublicKey)
		if !ok || k.X.Cmp(p.X) != 0 || k.Y.Cmp(p.Y) != 0 {
			return fmt.Errorf("私钥与证书公钥不匹配（EC）")
		}
	default:
		return fmt.Errorf("不支持的私钥类型 %T", priv)
	}
	return nil
}
