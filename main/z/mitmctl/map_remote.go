package mitmctl

import (
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/lqqyt2423/go-mitmproxy/proxy"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

const mapFromHeader = "X-Rocket-Map-From"

// MapRemoteAddon Charles 式远程映射：改写请求 URL，go-mitmproxy 会自动换独立上游连接。
type MapRemoteAddon struct {
	proxy.BaseAddon
	mu    sync.RWMutex
	rules []rvstore.MitmMapRemoteRule
}

func NewMapRemoteAddon(rules []rvstore.MitmMapRemoteRule) *MapRemoteAddon {
	return &MapRemoteAddon{rules: append([]rvstore.MitmMapRemoteRule(nil), rules...)}
}

func (a *MapRemoteAddon) SetRules(rules []rvstore.MitmMapRemoteRule) {
	a.mu.Lock()
	a.rules = append([]rvstore.MitmMapRemoteRule(nil), rules...)
	a.mu.Unlock()
}

func (a *MapRemoteAddon) Requestheaders(f *proxy.Flow) {
	if f == nil || f.Request == nil || f.Request.URL == nil {
		return
	}
	a.mu.RLock()
	rules := a.rules
	a.mu.RUnlock()
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if !matchMapFrom(f.Request.URL, rule) {
			continue
		}
		applyMapTo(f, rule)
		return
	}
}

func matchMapFrom(u *url.URL, rule rvstore.MitmMapRemoteRule) bool {
	if strings.TrimSpace(rule.FromHost) == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else if scheme == "http" {
			port = "80"
		}
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	query := u.RawQuery

	if fp := strings.TrimSpace(rule.FromProto); fp != "" && !strings.EqualFold(fp, scheme) {
		return false
	}
	if !hostEqual(host, strings.TrimSpace(rule.FromHost)) {
		return false
	}
	if fp := strings.TrimSpace(rule.FromPort); fp != "" && fp != port {
		return false
	}
	if !pathMatch(path, strings.TrimSpace(rule.FromPath)) {
		return false
	}
	if fq := rule.FromQuery; fq != "" && !queryMatch(query, fq) {
		return false
	}
	return true
}

func hostEqual(got, want string) bool {
	got = strings.ToLower(strings.TrimSpace(got))
	want = strings.ToLower(strings.TrimSpace(want))
	if got == want {
		return true
	}
	// 允许 FromHost 写成 host:port
	if h, p, err := net.SplitHostPort(want); err == nil {
		_ = p
		return got == strings.ToLower(h)
	}
	return false
}

func pathMatch(path, pattern string) bool {
	if pattern == "" {
		return true // 整个 host
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		if prefix == "" {
			return true
		}
		return strings.HasPrefix(path, prefix)
	}
	return path == pattern
}

func queryMatch(got, want string) bool {
	want = strings.TrimPrefix(want, "?")
	if want == "" {
		return true
	}
	if got == want {
		return true
	}
	// 子集：要求 want 中的每个 k=v 都出现在 got
	wantVals, _ := url.ParseQuery(want)
	gotVals, _ := url.ParseQuery(got)
	for k, vs := range wantVals {
		gvs := gotVals[k]
		for _, v := range vs {
			found := false
			for _, gv := range gvs {
				if gv == v {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func applyMapTo(f *proxy.Flow, rule rvstore.MitmMapRemoteRule) {
	u := f.Request.URL
	orig := u.String()
	scheme := u.Scheme
	host := u.Hostname()
	port := u.Port()
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	query := u.RawQuery

	if v := strings.TrimSpace(rule.ToProto); v != "" {
		scheme = strings.ToLower(v)
	}
	if v := strings.TrimSpace(rule.ToHost); v != "" {
		host = v
	}
	if v := strings.TrimSpace(rule.ToPort); v != "" {
		port = v
	} else if strings.TrimSpace(rule.ToHost) != "" {
		// 换 host 且未指定 port：按新协议默认端口；若与默认相同则省略
		port = ""
	}
	if v := rule.ToPath; v != "" {
		path = applyPathRewrite(path, strings.TrimSpace(rule.FromPath), v)
	}
	if rule.ToQuery != "" {
		query = strings.TrimPrefix(rule.ToQuery, "?")
	}

	hostPort := host
	if port != "" && !isDefaultPort(scheme, port) {
		hostPort = net.JoinHostPort(host, port)
	} else if port != "" && strings.Contains(host, ":") {
		hostPort = net.JoinHostPort(host, port)
	}

	u.Scheme = scheme
	u.Host = hostPort
	u.Path = path
	u.RawPath = ""
	u.RawQuery = query

	if f.Request.Header == nil {
		f.Request.Header = make(map[string][]string)
	}
	f.Request.Header.Set(mapFromHeader, orig)
	if !rule.PreserveHost {
		f.Request.Header.Set("Host", hostPort)
	}
	// 强制走独立 client（防御性；引擎也会因 host/scheme 变化自动开启）
	f.UseSeparateClient = true
}

func applyPathRewrite(origPath, fromPattern, toPath string) string {
	toPath = strings.TrimSpace(toPath)
	if toPath == "" {
		return origPath
	}
	fromPattern = strings.TrimSpace(fromPattern)
	if strings.HasSuffix(fromPattern, "*") {
		prefix := strings.TrimSuffix(fromPattern, "*")
		rest := strings.TrimPrefix(origPath, prefix)
		if strings.HasSuffix(toPath, "*") {
			return strings.TrimSuffix(toPath, "*") + rest
		}
		return toPath
	}
	return toPath
}

func isDefaultPort(scheme, port string) bool {
	switch strings.ToLower(scheme) {
	case "https":
		return port == "443"
	case "http":
		return port == "80"
	default:
		return false
	}
}
