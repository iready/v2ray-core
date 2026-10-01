package mitmctl

import (
	"net"
	"sort"
	"strings"
)

// ConnectEndpoint 客户端可填的代理地址。
type ConnectEndpoint struct {
	Host      string `json:"host"`
	Port      string `json:"port"`
	Proxy     string `json:"proxy"` // host:port，HTTP 代理栏直接填
	Interface string `json:"interface,omitempty"`
	Local     bool   `json:"local,omitempty"` // 127.0.0.1
}

// ListenPort 从监听地址解析端口。
func ListenPort(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "19080"
	}
	if strings.HasPrefix(addr, ":") {
		return strings.TrimPrefix(addr, ":")
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "19080"
	}
	return port
}

// ListConnectEndpoints 本机可连 IP + 代理端口（含 127.0.0.1）。
func ListConnectEndpoints(listenAddr string) []ConnectEndpoint {
	port := ListenPort(listenAddr)
	out := []ConnectEndpoint{{
		Host:  "127.0.0.1",
		Port:  port,
		Proxy: net.JoinHostPort("127.0.0.1", port),
		Local: true,
	}}

	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	type cand struct {
		ep    ConnectEndpoint
		score int
	}
	var cands []cand
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		name := iface.Name
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			host := ip.String()
			score := 10
			lname := strings.ToLower(name)
			switch {
			case strings.HasPrefix(lname, "en"), strings.HasPrefix(lname, "eth"), strings.HasPrefix(lname, "wlan"), strings.HasPrefix(lname, "wi-fi"):
				score = 100
			case strings.Contains(lname, "utun"), strings.Contains(lname, "tun"), strings.HasPrefix(lname, "awdl"), strings.HasPrefix(lname, "llw"):
				score = 1
			}
			cands = append(cands, cand{
				ep: ConnectEndpoint{
					Host:      host,
					Port:      port,
					Proxy:     net.JoinHostPort(host, port),
					Interface: name,
				},
				score: score,
			})
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].score != cands[j].score {
			return cands[i].score > cands[j].score
		}
		return cands[i].ep.Host < cands[j].ep.Host
	})
	seen := map[string]bool{"127.0.0.1": true}
	for _, c := range cands {
		if seen[c.ep.Host] {
			continue
		}
		seen[c.ep.Host] = true
		out = append(out, c.ep)
	}
	return out
}
