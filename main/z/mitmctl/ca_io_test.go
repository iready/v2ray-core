package mitmctl

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestImportExportCARoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	m := &Manager{flows: NewFlowStore(8)}
	caDir := filepath.Join(dir, ".rv", "mitm")
	if err := os.MkdirAll(caDir, 0755); err != nil {
		t.Fatal(err)
	}

	bundle := mustTestRSABundle(t)
	b64 := base64.StdEncoding.EncodeToString(bundle)
	if err := m.ImportCAContent(b64, ""); err != nil {
		t.Fatalf("import: %v", err)
	}
	got, err := os.ReadFile(CABundlePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == "" {
		t.Fatal("empty bundle")
	}
	if _, err := os.Stat(CACertPath()); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeCAInputPEM(t *testing.T) {
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("k")})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("c")})
	raw := append(keyPEM, certPEM...)
	out, err := decodeCAInput(string(raw), "")
	if err != nil {
		t.Fatal(err)
	}
	k, c, err := splitCABundle(out)
	if err != nil || k == nil || c == nil {
		t.Fatalf("split: %v", err)
	}
}

func TestDecodeCAInputBase64PEM(t *testing.T) {
	bundle := mustTestRSABundle(t)
	b64 := base64.StdEncoding.EncodeToString(bundle)
	out, err := decodeCAInput(b64, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := splitCABundle(out); err != nil {
		t.Fatal(err)
	}
}

func TestDecodeCAInputP12NeedsPassword(t *testing.T) {
	der := []byte{0x30, 0x82, 0x01, 0x00}
	for i := 0; i < 20; i++ {
		der = append(der, byte(i))
	}
	b64 := base64.StdEncoding.EncodeToString(der)
	_, err := decodeCAInput(b64, "")
	if err == nil || !strings.Contains(err.Error(), "密码") {
		t.Fatalf("want password required, got %v", err)
	}
}

func TestRejectECPrivateKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	m := &Manager{flows: NewFlowStore(8)}
	_ = os.MkdirAll(filepath.Join(dir, ".rv", "mitm"), 0755)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	// Pair with a real RSA cert so cert parse isn't the failure mode — use matching is hard;
	// sanitize checks key type before requiring key/cert match for TLS use.
	rsaBundle := mustTestRSABundle(t)
	_, certBlock, err := splitCABundle(rsaBundle)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	certPEM := pem.EncodeToMemory(certBlock)
	err = m.ImportCAContent(string(append(keyPEM, certPEM...)), "")
	if err == nil || !strings.Contains(err.Error(), "RSA") {
		t.Fatalf("want RSA-only reject, got %v", err)
	}
}

func TestRejectNonRootCA(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		IsCA:         false,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	err = validateMITMRootCA(cert)
	if err == nil || !strings.Contains(err.Error(), "根 CA") {
		t.Fatalf("want root CA reject, got %v", err)
	}
}

func TestRejectAbsurdDates(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "bad"},
		NotBefore:    time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:     time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	err = validateMITMRootCA(cert)
	if err == nil {
		t.Fatal("want date reject")
	}
}

func mustTestRSABundle(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-mitm-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return append(
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...,
	)
}
