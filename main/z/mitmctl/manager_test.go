package mitmctl

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

func TestApplyStartStop(t *testing.T) {
	m := &Manager{flows: NewFlowStore(32)}
	profile := rvstore.DefaultMitmProfile()
	profile.Use = true
	profile.Addr = "127.0.0.1:19091"
	profile.WebAddr = ""
	if err := m.Apply(profile); err != nil {
		t.Fatalf("apply: %v", err)
	}
	defer func() { _ = m.Stop() }()

	deadline := time.Now().Add(2 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:19091", 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			last = nil
			break
		}
		last = err
		time.Sleep(50 * time.Millisecond)
	}
	if !m.Status().Running && last != nil {
		t.Fatalf("not running: %v status=%+v", last, m.Status())
	}
	pem, err := m.CACertPEM()
	if err != nil || len(pem) == 0 {
		t.Fatalf("ca: %v len=%d", err, len(pem))
	}
	if err := m.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if m.Status().Running {
		t.Fatal("still running after stop")
	}
}

func TestRecorderCapturesFlow(t *testing.T) {
	m := &Manager{flows: NewFlowStore(32)}
	profile := rvstore.DefaultMitmProfile()
	profile.Use = true
	profile.Addr = "127.0.0.1:19093"
	profile.WebAddr = ""
	profile.SslInsecure = true
	if err := m.Apply(profile); err != nil {
		t.Fatalf("apply: %v", err)
	}
	defer func() { _ = m.Stop() }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:19093", 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	proxyURL := "http://127.0.0.1:19093"
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	transport := &http.Transport{
		Proxy: http.ProxyURL(mustParseURL(proxyURL)),
	}
	client := &http.Client{Timeout: 10 * time.Second, Transport: transport}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("via mitm: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.Flows().Len() > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	list := m.Flows().List()
	if len(list) == 0 {
		t.Fatal("expected recorded flows")
	}
	d, ok := m.Flows().Get(list[0].ID)
	if !ok || d.Method == "" {
		t.Fatalf("detail missing: %+v", d)
	}
}

func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}
