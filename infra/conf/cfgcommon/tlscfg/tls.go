package tlscfg

import (
	"encoding/base64"
	"encoding/pem"
	"strings"

	"github.com/golang/protobuf/proto"
	"golang.org/x/crypto/pkcs12"

	"github.com/v2fly/v2ray-core/v5/common/platform/filesystem"
	"github.com/v2fly/v2ray-core/v5/infra/conf/cfgcommon"
	"github.com/v2fly/v2ray-core/v5/transport/internet/tls"
)

//go:generate go run github.com/v2fly/v2ray-core/v5/common/errors/errorgen

type TLSConfig struct {
	Insecure                         bool                  `json:"allowInsecure"`
	Certs                            []*TLSCertConfig      `json:"certificates"`
	ServerName                       string                `json:"serverName"`
	ALPN                             *cfgcommon.StringList `json:"alpn"`
	EnableSessionResumption          bool                  `json:"enableSessionResumption"`
	DisableSystemRoot                bool                  `json:"disableSystemRoot"`
	PinnedPeerCertificateChainSha256 *[]string             `json:"pinnedPeerCertificateChainSha256"`
	VerifyClientCertificate          bool                  `json:"verifyClientCertificate"`
	ECHConfig                        string                `json:"echConfig"`
	ECHDOHServer                     string                `json:"echDohServer"`
}

// Build implements Buildable.
func (c *TLSConfig) Build() (proto.Message, error) {
	config := new(tls.Config)
	config.Certificate = make([]*tls.Certificate, len(c.Certs))
	for idx, certConf := range c.Certs {
		cert, err := certConf.Build()
		if err != nil {
			return nil, err
		}
		config.Certificate[idx] = cert
	}
	serverName := c.ServerName
	config.AllowInsecure = c.Insecure
	config.VerifyClientCertificate = c.VerifyClientCertificate
	if len(c.ServerName) > 0 {
		config.ServerName = serverName
	}
	if c.ALPN != nil && len(*c.ALPN) > 0 {
		config.NextProtocol = []string(*c.ALPN)
	}
	config.EnableSessionResumption = c.EnableSessionResumption
	config.DisableSystemRoot = c.DisableSystemRoot

	if c.PinnedPeerCertificateChainSha256 != nil {
		config.PinnedPeerCertificateChainSha256 = [][]byte{}
		for _, v := range *c.PinnedPeerCertificateChainSha256 {
			hashValue, err := base64.StdEncoding.DecodeString(v)
			if err != nil {
				return nil, err
			}
			config.PinnedPeerCertificateChainSha256 = append(config.PinnedPeerCertificateChainSha256, hashValue)
		}
	}

	if c.ECHConfig != "" {
		ECHConfig, err := base64.StdEncoding.DecodeString(c.ECHConfig)
		if err != nil {
			return nil, newError("invalid ECH Config", c.ECHConfig)
		}
		config.EchConfig = ECHConfig
	}

	config.Ech_DOHserver = c.ECHDOHServer

	return config, nil
}

type TLSCertConfig struct {
	CertFile       string   `json:"certificateFile"`
	CertStr        []string `json:"certificate"`
	KeyFile        string   `json:"keyFile"`
	KeyStr         []string `json:"key"`
	Usage          string   `json:"usage"`
	Pkcs12Base64   string   `json:"pkcs12Base64"`
	Pkcs12Password string   `json:"pkcs12Password"`
}

// Build implements Buildable.
func (c *TLSCertConfig) Build() (*tls.Certificate, error) {
	certificate := new(tls.Certificate)

	if len(c.Pkcs12Base64) > 0 {
		if len(c.CertFile) > 0 || len(c.CertStr) > 0 || len(c.KeyFile) > 0 || len(c.KeyStr) > 0 {
			return nil, newError("pkcs12Base64 conflicts with certificate/key fields")
		}
		cert, key, err := buildFromPKCS12(c.Pkcs12Base64, c.Pkcs12Password)
		if err != nil {
			return nil, err
		}
		certificate.Certificate = cert
		certificate.Key = key

		usage, err := validateP12Usage(c.Usage)
		if err != nil {
			return nil, err
		}
		certificate.Usage = usage
		return certificate, nil
	}

	cert, err := readFileOrString(c.CertFile, c.CertStr)
	if err != nil {
		return nil, newError("failed to parse certificate").Base(err)
	}
	certificate.Certificate = cert

	if len(c.KeyFile) > 0 || len(c.KeyStr) > 0 {
		key, err := readFileOrString(c.KeyFile, c.KeyStr)
		if err != nil {
			return nil, newError("failed to parse key").Base(err)
		}
		certificate.Key = key
	}

	switch strings.ToLower(c.Usage) {
	case "encipherment":
		certificate.Usage = tls.Certificate_ENCIPHERMENT
	case "verify":
		certificate.Usage = tls.Certificate_AUTHORITY_VERIFY
	case "verifyclient":
		certificate.Usage = tls.Certificate_AUTHORITY_VERIFY_CLIENT
	case "issue":
		certificate.Usage = tls.Certificate_AUTHORITY_ISSUE
	default:
		certificate.Usage = tls.Certificate_ENCIPHERMENT
	}

	return certificate, nil
}

func readFileOrString(f string, s []string) ([]byte, error) {
	if len(f) > 0 {
		return filesystem.ReadFile(f)
	}
	if len(s) > 0 {
		return []byte(strings.Join(s, "\n")), nil
	}
	return nil, newError("both file and bytes are empty.")
}

func buildFromPKCS12(pkcs12Base64 string, password string) ([]byte, []byte, error) {
	pfxData, err := base64.StdEncoding.DecodeString(pkcs12Base64)
	if err != nil {
		return nil, nil, newError("failed to decode pkcs12 base64").Base(err)
	}

	blocks, err := pkcs12.ToPEM(pfxData, password)
	if err != nil {
		return nil, nil, newError("failed to decode pkcs12").Base(err)
	}

	var certPEM []byte
	var keyPEM []byte
	for _, block := range blocks {
		if block == nil {
			continue
		}
		encodedBlock := pem.EncodeToMemory(block)
		if len(encodedBlock) == 0 {
			continue
		}
		blockType := strings.ToUpper(block.Type)
		switch {
		case strings.Contains(blockType, "PRIVATE KEY"):
			if len(keyPEM) == 0 {
				keyPEM = encodedBlock
			}
		case strings.Contains(blockType, "CERTIFICATE"):
			certPEM = append(certPEM, encodedBlock...)
		}
	}

	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return nil, nil, newError("pkcs12 missing private key or certificate")
	}

	return certPEM, keyPEM, nil
}

func validateP12Usage(usage string) (tls.Certificate_Usage, error) {
	if usage == "" || strings.EqualFold(usage, "encipherment") {
		return tls.Certificate_ENCIPHERMENT, nil
	}
	return tls.Certificate_ENCIPHERMENT, newError("pkcs12 certificate only supports usage=encipherment")
}
