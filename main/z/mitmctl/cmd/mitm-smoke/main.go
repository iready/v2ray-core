package main

// 独立 smoke：不碰正在跑的 rocket / TUN / 系统代理。
// 仅本进程启 MITM，用 HTTP_PROXY 风格请求自测。

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/v2fly/v2ray-core/v5/main/z/mitmctl"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

func main() {
	addr := env("MITM_ADDR", "127.0.0.1:19091")
	web := env("MITM_WEB", "127.0.0.1:19092")
	upstream := env("MITM_UPSTREAM", "socks5://127.0.0.1:1091")
	target := env("MITM_TARGET", "https://www.baidu.com/")

	m := &mitmctl.Manager{}
	p := rvstore.DefaultMitmProfile()
	p.Use = true
	p.Addr = addr
	p.WebAddr = web
	p.Upstream = upstream
	p.SslInsecure = true

	if err := m.Apply(p); err != nil {
		fail("apply", err)
	}
	defer func() { _ = m.Stop() }()

	waitListen("http://"+web+"/", 3*time.Second)

	pem, err := m.CACertPEM()
	if err != nil || len(pem) < 50 {
		fail("ca", fmt.Errorf("%v len=%d", err, len(pem)))
	}
	fmt.Printf("ok ca_pem_bytes=%d path=%s\n", len(pem), mitmctl.CACertPath())

	proxyURL, err := url.Parse("http://" + addr)
	if err != nil {
		fail("proxy url", err)
	}
	client := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // 测试进程未安装 CA；真实浏览器需装 CA
			},
		},
	}
	resp, err := client.Get(target)
	if err != nil {
		fail("via mitm "+target, err)
	}
	defer resp.Body.Close()
	n, _ := io.Copy(io.Discard, resp.Body)
	fmt.Printf("ok via_mitm status=%d bytes=%d target=%s upstream=%s\n", resp.StatusCode, n, target, upstream)
	fmt.Printf("ok status=%+v\n", m.Status())
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func waitListen(u string, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		resp, err := http.Get(u)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func fail(step string, err error) {
	fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", step, err)
	os.Exit(1)
}
