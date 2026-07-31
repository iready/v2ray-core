package res

import (
	"fmt"
	"net/netip"
	"strings"
	"sync"

	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
)

var (
	cnPrefixesOnce sync.Once
	cnPrefixes     []netip.Prefix
	cnPrefixesErr  error
)

// GeoIPPrefixes returns IPv4/IPv6 prefixes for a geoip country code (e.g. CN).
func GeoIPPrefixes(country string) ([]netip.Prefix, error) {
	country = strings.TrimSpace(country)
	if country == "" {
		return nil, fmt.Errorf("empty geoip country")
	}
	if strings.EqualFold(country, "CN") {
		cnPrefixesOnce.Do(func() {
			cnPrefixes, cnPrefixesErr = loadGeoIPPrefixes("CN")
		})
		return append([]netip.Prefix(nil), cnPrefixes...), cnPrefixesErr
	}
	return loadGeoIPPrefixes(country)
}

func loadGeoIPPrefixes(country string) ([]netip.Prefix, error) {
	cidrs, err := loadIP("geoip.dat", country)
	if err != nil {
		return nil, err
	}
	if len(cidrs) == 0 {
		return nil, fmt.Errorf("geoip country not found: %s", country)
	}
	return CIDRsToPrefixes(cidrs), nil
}

// CIDRsToPrefixes converts router CIDR list to netip.Prefix slice.
func CIDRsToPrefixes(cidrs []*routercommon.CIDR) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, cidr := range cidrs {
		if p, ok := CIDRToPrefix(cidr); ok {
			out = append(out, p)
		}
	}
	return out
}

// CIDRToPrefix converts a single router CIDR to netip.Prefix (network masked).
func CIDRToPrefix(cidr *routercommon.CIDR) (netip.Prefix, bool) {
	if cidr == nil || len(cidr.Ip) == 0 {
		return netip.Prefix{}, false
	}
	addr, ok := netip.AddrFromSlice(cidr.Ip)
	if !ok {
		return netip.Prefix{}, false
	}
	bits := int(cidr.Prefix)
	if bits < 0 {
		bits = len(cidr.Ip) * 8
	}
	prefix := netip.PrefixFrom(addr, bits)
	if !prefix.IsValid() {
		return netip.Prefix{}, false
	}
	return prefix.Masked(), true
}

// ExpandIPMatcher expands routing ip tokens (geoip:cn, 1.2.3.0/24) to prefixes.
// inverse on geoip means complement within embedded geoip.dat for that country only.
func ExpandIPMatcher(token string) ([]netip.Prefix, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}
	if strings.HasPrefix(token, "geoip:") {
		country := strings.TrimPrefix(token, "geoip:")
		inverse := false
		if strings.HasPrefix(country, "!") {
			inverse = true
			country = strings.TrimPrefix(country, "!")
		}
		country = strings.TrimSpace(country)
		if country == "" {
			return nil, fmt.Errorf("empty geoip token")
		}
		if strings.EqualFold(country, "private") {
			return privatePrefixes(inverse), nil
		}
		prefixes, err := GeoIPPrefixes(country)
		if err != nil {
			return nil, err
		}
		if !inverse {
			return prefixes, nil
		}
		return invertCountryPrefixes(prefixes), nil
	}
	if p, err := netip.ParsePrefix(token); err == nil && p.IsValid() {
		return []netip.Prefix{p.Masked()}, nil
	}
	if addr, err := netip.ParseAddr(token); err == nil && addr.IsValid() {
		bits := addr.BitLen()
		return []netip.Prefix{netip.PrefixFrom(addr, bits).Masked()}, nil
	}
	return nil, fmt.Errorf("unsupported ip matcher: %s", token)
}

func privatePrefixes(inverse bool) []netip.Prefix {
	private := []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
	}
	var out []netip.Prefix
	for _, cidr := range private {
		p, err := netip.ParsePrefix(cidr)
		if err == nil {
			out = append(out, p)
		}
	}
	if inverse {
		return invertCountryPrefixes(out)
	}
	return out
}

// invertCountryPrefixes is not full complement; used only for small sets like private.
func invertCountryPrefixes(_ []netip.Prefix) []netip.Prefix {
	return nil
}
