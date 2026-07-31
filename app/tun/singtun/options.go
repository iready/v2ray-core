package singtun

import (
	"fmt"
	"net/netip"
	"runtime"

	stun "github.com/sagernet/sing-tun"
	"github.com/v2fly/v2ray-core/v5/app/proxyman"
	"github.com/v2fly/v2ray-core/v5/app/router/routercommon"
)

const defaultStack = "mixed"

// ConfigInput mirrors v2ray services.tun fields used by sing-tun.
type ConfigInput struct {
	Name             string
	MTU              uint32
	Tag              string
	UserLevel        uint32
	IPs              []*routercommon.CIDR
	Routes           []*routercommon.CIDR
	SniffingSettings *proxyman.SniffingConfig
}

func BuildOptions(input ConfigInput) (stun.Options, string, ConfigInput, []netip.Prefix, []string) {
	platform := takePlatform()

	name := input.Name
	if platform.InterfaceName != "" {
		name = platform.InterfaceName
	}
	if name == "" || name == "tun0" {
		name = stun.CalculateInterfaceName("")
	} else if runtime.GOOS == "darwin" && name != "" {
		name = stun.CalculateInterfaceName(name)
	}

	mtu := input.MTU
	if mtu == 0 {
		mtu = 1500
	}

	inet4, inet6 := cidrListToInterfacePrefixes(input.IPs)
	if len(inet4) == 0 && len(inet6) == 0 {
		inet4 = []netip.Prefix{netip.MustParsePrefix(
			fmt.Sprintf("%s/%d", DefaultTunIPv4, DefaultTunPrefix),
		)}
	}

	route4, route6 := cidrListToRoutePrefixes(input.Routes)
	autoRoute := platform.AutoRoute
	if !platform.ExternalConfiguration && platform.FileDescriptor == 0 && (len(route4) > 0 || len(route6) > 0) {
		autoRoute = true
	}

	exclude := MergeRouteExcludes(platform.RouteExcludeAddresses)

	opts := stun.Options{
		Name:                     name,
		MTU:                      mtu,
		Inet4Address:             inet4,
		Inet6Address:             inet6,
		Inet4RouteAddress:        route4,
		Inet6RouteAddress:        route6,
		Inet4RouteExcludeAddress: filterPrefixes(exclude, func(p netip.Prefix) bool { return p.Addr().Is4() }),
		Inet6RouteExcludeAddress: filterPrefixes(exclude, func(p netip.Prefix) bool { return p.Addr().Is6() }),
		AutoRoute:                autoRoute,
		// Windows StrictRoute 走 WFP，与出站 bindToDevice 叠用会黑洞回程；路由排除 + bind 已足够防环。
		StrictRoute:               autoRoute && runtime.GOOS != "windows",
		FileDescriptor:            platform.FileDescriptor,
		EXP_ExternalConfiguration: platform.ExternalConfiguration || platform.FileDescriptor > 0,
		Logger:                    nopLogger{},
	}
	if len(inet4) > 0 && (autoRoute || platform.ExternalConfiguration || platform.FileDescriptor > 0) {
		// Windows/Linux：系统/resolv DNS=127.0.0.1，由本机 :53 dokodemo 进 dns-out。
		// Darwin：ExternalConfiguration，DNS 由 helper SET_DNS→127.0.0.1→53535。
		// 注意：Linux 不可把 DNS 指到 TUN 地址本身——对端是本机地址时 UDP 不进 gVisor 劫持路径。
		if runtime.GOOS == "darwin" {
			opts.DNSServers = []netip.Addr{TunDNSAddr(inet4)}
		} else {
			opts.DNSServers = []netip.Addr{netip.AddrFrom4([4]byte{127, 0, 0, 1})}
		}
	}

	return opts, defaultStack, input, exclude, append([]string(nil), platform.BypassHosts...)
}

func cidrListToInterfacePrefixes(cidrs []*routercommon.CIDR) (inet4, inet6 []netip.Prefix) {
	// 接口地址保留主机位（如 172.19.0.1/30），不能 Masked 成网络地址 172.19.0.0。
	return cidrListToPrefixes(cidrs, false)
}

func cidrListToRoutePrefixes(cidrs []*routercommon.CIDR) (inet4, inet6 []netip.Prefix) {
	return cidrListToPrefixes(cidrs, true)
}

func cidrListToPrefixes(cidrs []*routercommon.CIDR, maskNetwork bool) (inet4, inet6 []netip.Prefix) {
	for _, cidr := range cidrs {
		if cidr == nil || len(cidr.Ip) == 0 {
			continue
		}
		bits := int(cidr.Prefix)
		if bits < 0 {
			bits = len(cidr.Ip) * 8
		}
		addr, ok := netip.AddrFromSlice(cidr.Ip)
		if !ok {
			continue
		}
		prefix := netip.PrefixFrom(addr, bits)
		if !prefix.IsValid() {
			continue
		}
		if maskNetwork {
			prefix = prefix.Masked()
		}
		if addr.Is4() {
			inet4 = append(inet4, prefix)
		} else if addr.Is6() {
			inet6 = append(inet6, prefix)
		}
	}
	return
}

func filterPrefixes(prefixes []netip.Prefix, keep func(netip.Prefix) bool) []netip.Prefix {
	var out []netip.Prefix
	for _, p := range prefixes {
		if keep(p) {
			out = append(out, p)
		}
	}
	return out
}
