package tunctl

import (
	"net"
	"strings"
	"sync"

	"github.com/v2fly/v2ray-core/v5/transport/internet"
)

var (
	dialBindMu    sync.RWMutex
	dialBindIface string
)

func init() {
	_ = internet.RegisterDialerController(func(network, address string, fd uintptr) error {
		if isLoopbackAddr(address) {
			return nil
		}
		return withDialBindIface(func(iface string) error {
			return bindOutboundToInterface(network, address, fd, iface)
		})
	})
	// UDP 出站走 ListenPacket(:0)，须绑物理网卡；固定端口 inbound 绝不能绑——
	// Darwin 上 IP_BOUND_IF 会把本机 127.0.0.1 的 SYN-ACK 打到 enX，客户端永远 SYN_SENT。
	_ = internet.RegisterListenerController(func(network, address string, fd uintptr) error {
		if !isEphemeralListenAddr(address) || isLoopbackAddr(address) {
			return nil
		}
		return withDialBindIface(func(iface string) error {
			return bindOutboundToInterface(network, address, fd, iface)
		})
	})
}

func withDialBindIface(fn func(iface string) error) error {
	dialBindMu.RLock()
	iface := dialBindIface
	dialBindMu.RUnlock()
	if iface == "" {
		return nil
	}
	return fn(iface)
}

func isLoopbackAddr(address string) bool {
	host := dialHost(address)
	if host == "" || strings.EqualFold(host, "localhost") {
		return host != ""
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func dialHost(address string) string {
	host := address
	if h, _, err := net.SplitHostPort(address); err == nil {
		host = h
	}
	return strings.TrimSpace(host)
}

// isEphemeralListenAddr 识别出站 UDP ListenPacket（port=0）；固定端口视为 inbound。
func isEphemeralListenAddr(address string) bool {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	return port == "0"
}

// EnableDialBind 对齐 sing-box route.auto_detect_interface：TUN 活跃时全局绑定出站物理网卡。
func EnableDialBind(iface string) {
	dialBindMu.Lock()
	dialBindIface = strings.TrimSpace(iface)
	dialBindMu.Unlock()
}

// DisableDialBind 关闭出站绑网卡（TUN 关闭或降级时调用）。
func DisableDialBind() {
	EnableDialBind("")
}
