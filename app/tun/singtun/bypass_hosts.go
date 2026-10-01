package singtun

import (
	"net/netip"
	"strings"
	"sync"
)

// hostBypassIndex 把 bypassHosts 预编译成 IP hash + prefix set，避免 PrepareConnection 热路径反复 Parse/扫表。
type hostBypassIndex struct {
	ips      map[netip.Addr]struct{}
	prefixes *prefixSet
	names    []string
	dns      bypassDNSCache
	mu       sync.Mutex
}

func buildHostBypassIndex(hosts []string) *hostBypassIndex {
	idx := &hostBypassIndex{
		ips: make(map[netip.Addr]struct{}),
	}
	var prefixes []netip.Prefix
	seenName := make(map[string]struct{})
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		if ip, err := netip.ParseAddr(host); err == nil {
			idx.ips[ip] = struct{}{}
			continue
		}
		if strings.Contains(host, "/") {
			if prefix, err := netip.ParsePrefix(host); err == nil {
				prefixes = append(prefixes, prefix)
				continue
			}
		}
		if _, ok := seenName[host]; ok {
			continue
		}
		seenName[host] = struct{}{}
		idx.names = append(idx.names, host)
	}
	idx.prefixes = newPrefixSet(prefixes)
	return idx
}

func (idx *hostBypassIndex) contains(addr netip.Addr) bool {
	if idx == nil {
		return false
	}
	idx.mu.Lock()
	_, ok := idx.ips[addr]
	idx.mu.Unlock()
	if ok {
		return true
	}
	if idx.prefixes.Contains(addr) {
		return true
	}
	for _, name := range idx.names {
		resolved := idx.dns.resolve(name)
		if len(resolved) == 0 {
			continue
		}
		idx.mu.Lock()
		for _, a := range resolved {
			idx.ips[a] = struct{}{}
		}
		_, ok = idx.ips[addr]
		idx.mu.Unlock()
		if ok {
			return true
		}
	}
	return false
}
