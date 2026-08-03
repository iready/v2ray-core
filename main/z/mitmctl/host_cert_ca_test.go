package mitmctl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

type stubCA struct{}

func (stubCA) GetRootCA() *x509.Certificate { return &x509.Certificate{} }
func (stubCA) GetCert(cn string) (*tls.Certificate, error) {
	return &tls.Certificate{Certificate: [][]byte{[]byte("base-" + cn)}}, nil
}

func TestHostCertCAExactAndSuffix(t *testing.T) {
	certPEM, keyPEM := mustGenRSALeaf(t, "api.example.com")
	ca, err := NewHostCertCA(stubCA{}, []rvstore.MitmHostCertRule{
		{Enabled: true, Host: "api.example.com", CertPEM: certPEM, KeyPEM: keyPEM},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ca.GetCert("api.example.com")
	if err != nil || got == nil || len(got.Certificate) == 0 || len(got.Certificate[0]) < 32 {
		t.Fatalf("exact miss: %v %#v", err, got)
	}
	sub, err := ca.GetCert("x.api.example.com")
	if err != nil || sub == nil || len(sub.Certificate[0]) < 32 {
		t.Fatalf("suffix miss: %v", err)
	}
	other, err := ca.GetCert("other.com")
	if err != nil {
		t.Fatal(err)
	}
	if string(other.Certificate[0]) != "base-other.com" {
		t.Fatalf("expected base CA, got %q", other.Certificate[0])
	}
}

func TestHostCertCAWildcard(t *testing.T) {
	certPEM, keyPEM := mustGenRSALeaf(t, "*.cdn.example.com")
	ca, err := NewHostCertCA(stubCA{}, []rvstore.MitmHostCertRule{
		{Enabled: true, Host: "*.cdn.example.com", CertPEM: certPEM, KeyPEM: keyPEM},
	})
	if err != nil {
		t.Fatal(err)
	}
	hit, err := ca.GetCert("a.cdn.example.com")
	if err != nil || hit == nil || len(hit.Certificate[0]) < 32 {
		t.Fatalf("wildcard miss: %v", err)
	}
	// exact apex of wildcard pattern should also match via suffix equality
	apex, err := ca.GetCert("cdn.example.com")
	if err != nil || apex == nil || len(apex.Certificate[0]) < 32 {
		t.Fatalf("apex miss: %v", err)
	}
}

func TestParseHostCertMaterialRSA(t *testing.T) {
	certPEM, keyPEM := mustGenRSALeaf(t, "leaf.test")
	bundle := keyPEM + certPEM
	c, k, err := ParseHostCertMaterial(bundle, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadHostTLSCertificate(c, k); err != nil {
		t.Fatal(err)
	}
}

func TestParseHostCertMaterialEC(t *testing.T) {
	certPEM, keyPEM := mustGenECLeaf(t, "ec.leaf.test")
	c, k, err := ParseHostCertMaterial(keyPEM+certPEM, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loadHostTLSCertificate(c, k); err != nil {
		t.Fatal(err)
	}
}

func mustGenRSALeaf(t *testing.T, cn string) (certPEM, keyPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return mustLeafPEM(t, cn, key, &key.PublicKey)
}

func mustGenECLeaf(t *testing.T, cn string) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return mustLeafPEM(t, cn, key, &key.PublicKey)
}

func mustLeafPEM(t *testing.T, cn string, key any, pub any) (certPEM, keyPEM string) {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{cn},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))
}
