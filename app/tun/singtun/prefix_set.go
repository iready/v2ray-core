package singtun

import (
	"net/netip"

	"go4.org/netipx"
)

type prefixSet struct {
	ip4 *netipx.IPSet
	ip6 *netipx.IPSet
}

func newPrefixSet(prefixes []netip.Prefix) *prefixSet {
	if len(prefixes) == 0 {
		return nil
	}
	var b4, b6 netipx.IPSetBuilder
	has4, has6 := false, false
	for _, p := range prefixes {
		if !p.IsValid() {
			continue
		}
		p = p.Masked()
		if p.Addr().Is4() {
			b4.AddPrefix(p)
			has4 = true
		} else if p.Addr().Is6() {
			b6.AddPrefix(p)
			has6 = true
		}
	}
	if !has4 && !has6 {
		return nil
	}
	ps := &prefixSet{}
	if has4 {
		ps.ip4, _ = b4.IPSet()
	}
	if has6 {
		ps.ip6, _ = b6.IPSet()
	}
	return ps
}

func (s *prefixSet) Contains(addr netip.Addr) bool {
	if s == nil {
		return false
	}
	if addr.Is4() && s.ip4 != nil {
		return s.ip4.Contains(addr)
	}
	if addr.Is6() && s.ip6 != nil {
		return s.ip6.Contains(addr)
	}
	return false
}
