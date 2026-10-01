package res

import (
	"fmt"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
)

// GeoSiteHosts 返回 geosite 列表中可解析的显式主机名（full / domain 类型）。
func GeoSiteHosts(list string) ([]string, error) {
	list = strings.TrimSpace(list)
	if list == "" {
		return nil, fmt.Errorf("empty geosite list")
	}
	domains, err := loadSite("geosite.dat", list)
	if err != nil {
		return nil, err
	}
	if len(domains) == 0 {
		return nil, fmt.Errorf("geosite list not found: %s", list)
	}
	seen := make(map[string]struct{})
	var out []string
	for _, d := range domains {
		if d == nil {
			continue
		}
		host := geositeDomainHost(d)
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	return out, nil
}

func geositeDomainHost(d *routercommon.Domain) string {
	switch d.Type {
	case routercommon.Domain_Full:
		return strings.TrimSpace(d.Value)
	case routercommon.Domain_Plain, routercommon.Domain_RootDomain:
		v := strings.TrimSpace(d.Value)
		if v == "" || strings.Contains(v, "*") {
			return ""
		}
		return v
	default:
		return ""
	}
}
