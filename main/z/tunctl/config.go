package tunctl

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
)

type tunCIDR struct {
	IP     []int `json:"ip"`
	Prefix int   `json:"prefix"`
}

// ConfigTunParams parsed from Rocket 下发的 runtime v2fly JSON services.tun.
type ConfigTunParams struct {
	Name           string    `json:"name"`
	MTU            uint32    `json:"mtu"`
	Tag            string    `json:"tag"`
	RouteMode      string    `json:"route_mode"`
	RouteModeCamel string    `json:"routeMode"`
	Routes         []tunCIDR `json:"routes"`
	IPs            []tunCIDR `json:"ips"`
}

func (p *ConfigTunParams) effectiveRouteMode() string {
	if m := strings.TrimSpace(p.RouteMode); m != "" {
		return m
	}
	return strings.TrimSpace(p.RouteModeCamel)
}

// FirstInet4 returns the first IPv4 CIDR from services.tun.ips.
func (p ConfigTunParams) FirstInet4() (ip string, prefix int) {
	for _, c := range p.IPs {
		if len(c.IP) != 4 {
			continue
		}
		pr := c.Prefix
		if pr <= 0 {
			pr = singtun.DefaultTunPrefix
		}
		return fmt.Sprintf("%d.%d.%d.%d", c.IP[0], c.IP[1], c.IP[2], c.IP[3]), pr
	}
	return singtun.DefaultTunIPv4, singtun.DefaultTunPrefix
}

// HasTUNService reports whether raw v2fly JSON enables services.tun.
func HasTUNService(raw string) bool {
	var doc struct {
		Services struct {
			Tun json.RawMessage `json:"tun"`
		} `json:"services"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return strings.Contains(raw, `"tun"`) && strings.Contains(raw, `"services"`)
	}
	return len(doc.Services.Tun) > 0 && string(doc.Services.Tun) != "null"
}

func defaultTunName() string {
	if runtime.GOOS == "darwin" {
		return "utun"
	}
	return "tun0"
}

// ParseTUNParams extracts tun fields from generated runtime config JSON.
func ParseTUNParams(raw string) ConfigTunParams {
	var doc struct {
		Services struct {
			Tun ConfigTunParams `json:"tun"`
		} `json:"services"`
	}
	_ = json.Unmarshal([]byte(raw), &doc)
	p := doc.Services.Tun
	if p.Name == "" {
		p.Name = defaultTunName()
	}
	if p.MTU == 0 {
		p.MTU = 1500
	}
	return p
}
