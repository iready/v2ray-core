package wire

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/pkcs12"

	certembed "github.com/v2fly/v2ray-core/v5/main/z/wire/certs"
)

// TLSOptions 命令行 mTLS 配置（仅 wss 生效）。
type TLSOptions struct {
	Mode       string // off | embed | file
	CAFile     string
	CertFile   string
	KeyFile    string
	P12File    string
	P12Pass    string
	ServerName string
	Insecure   bool
}

// TLSConfig 解析后的 mTLS 材料。
type TLSConfig struct {
	RootCAPEM          []byte
	ClientCertPEM      []byte
	ClientKeyPEM       []byte
	ClientP12          []byte
	ClientP12Pass      string
	ServerName         string
	InsecureSkipVerify bool
}

func (o TLSOptions) ForURL(wsURL string) (*TLSConfig, error) {
	if !IsWSS(wsURL) {
		return nil, nil
	}
	mode := strings.ToLower(strings.TrimSpace(o.Mode))
	if mode == "" || mode == "off" {
		return nil, nil
	}
	var cfg *TLSConfig
	var err error
	switch mode {
	case "embed":
		cfg, err = tlsFromEmbed(o.ServerName)
	case "file":
		cfg, err = tlsFromFiles(o.CAFile, o.CertFile, o.KeyFile, o.P12File, o.P12Pass, o.ServerName)
	default:
		return nil, fmt.Errorf("wire: unknown mtls mode %q (off|embed|file)", o.Mode)
	}
	if err != nil {
		return nil, err
	}
	cfg.InsecureSkipVerify = o.Insecure
	return cfg, nil
}

func tlsFromEmbed(serverName string) (*TLSConfig, error) {
	if len(certembed.RootCAPEM) == 0 || len(certembed.ClientP12) == 0 {
		return nil, fmt.Errorf("wire: embedded certs missing")
	}
	pass := strings.TrimSpace(string(certembed.ClientP12Pass))
	if pass == "" {
		return nil, fmt.Errorf("wire: embedded client.p12.pass is empty")
	}
	return &TLSConfig{
		RootCAPEM:     certembed.RootCAPEM,
		ClientP12:     certembed.ClientP12,
		ClientP12Pass: pass,
		ServerName:    serverName,
	}, nil
}

func tlsFromFiles(caFile, certFile, keyFile, p12File, p12Pass, serverName string) (*TLSConfig, error) {
	caFile = resolveFilePath(strings.TrimSpace(caFile))
	certFile = resolveFilePath(strings.TrimSpace(certFile))
	keyFile = resolveFilePath(strings.TrimSpace(keyFile))
	p12File = resolveFilePath(strings.TrimSpace(p12File))
	if caFile == "" {
		return nil, fmt.Errorf("wire: mtls requires ca file")
	}
	rootPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("wire: read mtls ca: %w", err)
	}
	cfg := &TLSConfig{
		RootCAPEM:  rootPEM,
		ServerName: serverName,
	}
	if certFile != "" && keyFile != "" {
		cfg.ClientCertPEM, err = os.ReadFile(certFile)
		if err != nil {
			return nil, fmt.Errorf("wire: read mtls cert: %w", err)
		}
		cfg.ClientKeyPEM, err = os.ReadFile(keyFile)
		if err != nil {
			return nil, fmt.Errorf("wire: read mtls key: %w", err)
		}
		return cfg, nil
	}
	if p12File == "" {
		return nil, fmt.Errorf("wire: mtls requires cert+key or p12")
	}
	if strings.TrimSpace(p12Pass) == "" {
		return nil, fmt.Errorf("wire: mtls p12 requires password")
	}
	cfg.ClientP12, err = os.ReadFile(p12File)
	if err != nil {
		return nil, fmt.Errorf("wire: read mtls p12: %w", err)
	}
	cfg.ClientP12Pass = p12Pass
	return cfg, nil
}

func resolveFilePath(path string) string {
	path = filepath.Clean(path)
	if filepath.IsAbs(path) {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path)
}

func (tc *TLSConfig) goTLS(host string) (*tls.Config, error) {
	if tc == nil {
		return nil, nil
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(tc.RootCAPEM) {
		return nil, fmt.Errorf("wire: invalid root CA PEM")
	}
	clientCert, err := tc.clientCertificate()
	if err != nil {
		return nil, err
	}
	sni := strings.TrimSpace(tc.ServerName)
	if sni == "" {
		sni = host
	}
	return &tls.Config{
		RootCAs:            pool,
		Certificates:       []tls.Certificate{clientCert},
		ServerName:         sni,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: tc.InsecureSkipVerify,
	}, nil
}

func (tc *TLSConfig) clientCertificate() (tls.Certificate, error) {
	if len(tc.ClientCertPEM) > 0 && len(tc.ClientKeyPEM) > 0 {
		return tls.X509KeyPair(tc.ClientCertPEM, tc.ClientKeyPEM)
	}
	if len(tc.ClientP12) > 0 {
		return certFromP12(tc.ClientP12, tc.ClientP12Pass)
	}
	return tls.Certificate{}, fmt.Errorf("wire: missing client cert material")
}

func certFromP12(pfx []byte, password string) (tls.Certificate, error) {
	blocks, err := pkcs12.ToPEM(pfx, password)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("wire: decode p12: %w", err)
	}
	var certPEM, keyPEM []byte
	for _, block := range blocks {
		if block == nil {
			continue
		}
		encoded := pem.EncodeToMemory(block)
		if len(encoded) == 0 {
			continue
		}
		switch {
		case strings.Contains(strings.ToUpper(block.Type), "PRIVATE KEY") && len(keyPEM) == 0:
			keyPEM = encoded
		case strings.Contains(strings.ToUpper(block.Type), "CERTIFICATE"):
			certPEM = append(certPEM, encoded...)
		}
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return tls.Certificate{}, fmt.Errorf("wire: p12 missing cert or key")
	}
	return tls.X509KeyPair(certPEM, keyPEM)
}
