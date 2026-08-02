package mitmctl

import (
	"net/url"
	"testing"

	"github.com/lqqyt2423/go-mitmproxy/proxy"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

func TestMatchMapFromHostAndPathPrefix(t *testing.T) {
	u, _ := url.Parse("https://api.xy.mifengbangzu.com/renter/user/wechatInfo?x=1")
	rule := rvstore.MitmMapRemoteRule{
		Enabled:  true,
		FromHost: "api.xy.mifengbangzu.com",
		FromPath: "/renter/*",
	}
	if !matchMapFrom(u, rule) {
		t.Fatal("expected match")
	}
	rule.FromPath = "/other/*"
	if matchMapFrom(u, rule) {
		t.Fatal("should not match")
	}
}

func TestApplyMapToHost(t *testing.T) {
	u, _ := url.Parse("https://api.xy.mifengbangzu.com/renter/user/info")
	f := &proxy.Flow{Request: &proxy.Request{URL: u, Header: map[string][]string{"Host": {"api.xy.mifengbangzu.com"}}}}
	rule := rvstore.MitmMapRemoteRule{
		Enabled:  true,
		FromHost: "api.xy.mifengbangzu.com",
		ToHost:   "127.0.0.1",
		ToPort:   "8080",
		ToProto:  "http",
	}
	applyMapTo(f, rule)
	if f.Request.URL.Scheme != "http" || f.Request.URL.Host != "127.0.0.1:8080" {
		t.Fatalf("got %s", f.Request.URL.String())
	}
	if f.Request.Header.Get("Host") != "127.0.0.1:8080" {
		t.Fatalf("host header %s", f.Request.Header.Get("Host"))
	}
	if f.Request.Header.Get(mapFromHeader) == "" {
		t.Fatal("missing map-from header")
	}
	if !f.UseSeparateClient {
		t.Fatal("want separate client")
	}
}

func TestApplyMapPreserveHost(t *testing.T) {
	u, _ := url.Parse("https://api.example.com/v1/a")
	f := &proxy.Flow{Request: &proxy.Request{URL: u, Header: map[string][]string{"Host": {"api.example.com"}}}}
	rule := rvstore.MitmMapRemoteRule{
		Enabled:      true,
		FromHost:     "api.example.com",
		ToHost:       "staging.example.com",
		PreserveHost: true,
	}
	applyMapTo(f, rule)
	if f.Request.Header.Get("Host") != "api.example.com" {
		t.Fatalf("preserve host failed: %s", f.Request.Header.Get("Host"))
	}
}

func TestPathRewriteWildcard(t *testing.T) {
	got := applyPathRewrite("/renter/user/info", "/renter/*", "/mock/*")
	if got != "/mock/user/info" {
		t.Fatalf("got %s", got)
	}
}
