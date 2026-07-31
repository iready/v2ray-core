package singtun

import (
	"net"
	"net/netip"
	"sync"
	"time"
)

const bypassDNSCacheTTL = 5 * time.Minute

type bypassDNSCache struct {
	mu      sync.RWMutex
	entries map[string]bypassDNSEntry
}

type bypassDNSEntry struct {
	addrs  []netip.Addr
	expiry time.Time
}

func (c *bypassDNSCache) lookup(host string) ([]netip.Addr, bool) {
	c.mu.RLock()
	entry, ok := c.entries[host]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expiry) {
		return nil, false
	}
	return entry.addrs, true
}

func (c *bypassDNSCache) store(host string, addrs []netip.Addr) {
	if len(addrs) == 0 {
		return
	}
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[string]bypassDNSEntry)
	}
	copied := append([]netip.Addr(nil), addrs...)
	c.entries[host] = bypassDNSEntry{addrs: copied, expiry: time.Now().Add(bypassDNSCacheTTL)}
	c.mu.Unlock()
}

func (c *bypassDNSCache) resolve(host string) []netip.Addr {
	if addrs, ok := c.lookup(host); ok {
		return addrs
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil
	}
	var addrs []netip.Addr
	for _, ip := range ips {
		if parsed, ok := netip.AddrFromSlice(ip); ok {
			addrs = append(addrs, parsed)
		}
	}
	c.store(host, addrs)
	return addrs
}
