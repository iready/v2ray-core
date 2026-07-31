package singtun

import (
	"net/netip"
	"sync"
	"time"
)

const bypassDecisionCacheTTL = 10 * time.Minute

type bypassDecisionCache struct {
	mu      sync.RWMutex
	entries map[netip.Addr]bypassDecisionEntry
}

type bypassDecisionEntry struct {
	bypass bool
	expiry time.Time
}

var globalBypassCache bypassDecisionCache

func (c *bypassDecisionCache) lookup(addr netip.Addr) (bypass bool, ok bool) {
	if !addr.IsValid() {
		return false, false
	}
	now := time.Now()
	c.mu.RLock()
	entry, found := c.entries[addr]
	c.mu.RUnlock()
	if !found {
		return false, false
	}
	if now.After(entry.expiry) {
		c.mu.Lock()
		if e, still := c.entries[addr]; still && now.After(e.expiry) {
			delete(c.entries, addr)
		}
		c.mu.Unlock()
		return false, false
	}
	return entry.bypass, true
}

func (c *bypassDecisionCache) store(addr netip.Addr, bypass bool) {
	if !addr.IsValid() {
		return
	}
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[netip.Addr]bypassDecisionEntry)
	}
	c.entries[addr] = bypassDecisionEntry{bypass: bypass, expiry: time.Now().Add(bypassDecisionCacheTTL)}
	c.mu.Unlock()
}

func (c *bypassDecisionCache) clear() {
	c.mu.Lock()
	c.entries = nil
	c.mu.Unlock()
}

// ClearBypassDecisionCache 在 TUN 重连或配置变更时清空直连决策缓存。
func ClearBypassDecisionCache() {
	globalBypassCache.clear()
}
