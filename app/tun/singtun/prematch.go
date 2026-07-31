package singtun

import (
	"context"
	"sync"

	v2net "github.com/v2fly/v2ray-core/v5/common/net"
	"github.com/v2fly/v2ray-core/v5/common/session"
	routing_session "github.com/v2fly/v2ray-core/v5/features/routing/session"

	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/v2fly/v2ray-core/v5/common"
)

var (
	proxyListModeMu sync.RWMutex
	proxyListMode   bool
)

// SetProxyListMode 启用 proxy_list PreMatch（sing-box 式 userspace 分流）。
func SetProxyListMode(enabled bool) {
	proxyListModeMu.Lock()
	proxyListMode = enabled
	proxyListModeMu.Unlock()
}

func proxyListModeEnabled() bool {
	proxyListModeMu.RLock()
	defer proxyListModeMu.RUnlock()
	return proxyListMode
}

// shouldBypassPreMatch sing-tun gvisor TCP 对 ErrBypass 会直接丢连接，国内/直连须走 userspace。
func (h *Handler) shouldBypassPreMatch(network string, source, destination M.Socksaddr) bool {
	_ = network
	_ = source
	_ = destination
	return false
}

func (h *Handler) resolveOutboundTag(network string, source, destination M.Socksaddr) (tag string, noRule bool) {
	ctx := h.buildRoutingContext(network, source, destination)
	route, err := h.router.PickRoute(routing_session.AsRoutingContext(ctx))
	if err == nil {
		return route.GetOutboundTag(), false
	}
	if err != common.ErrNoClue {
		return "", false
	}
	if h.outbound != nil {
		if def := h.outbound.GetDefaultHandler(); def != nil {
			return def.Tag(), true
		}
	}
	return "", true
}

func (h *Handler) buildRoutingContext(network string, source, destination M.Socksaddr) context.Context {
	var v2Network v2net.Network
	switch network {
	case N.NetworkUDP:
		v2Network = v2net.Network_UDP
	default:
		v2Network = v2net.Network_TCP
	}
	target := socksaddrToDestination(destination, v2Network)
	ctx := session.ContextWithInbound(h.ctx, &session.Inbound{
		Tag:    h.tag,
		Source: socksaddrToDestination(source, v2Network),
	})
	ctx = session.ContextWithOutbound(ctx, &session.Outbound{Target: target})
	return h.withSniffing(ctx)
}

func socksaddrToDestination(addr M.Socksaddr, network v2net.Network) v2net.Destination {
	if network == v2net.Network_UDP {
		return v2net.UDPDestination(v2net.IPAddress(addr.Addr.AsSlice()), v2net.Port(addr.Port))
	}
	return v2net.TCPDestination(v2net.IPAddress(addr.Addr.AsSlice()), v2net.Port(addr.Port))
}
