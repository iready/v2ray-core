package tunctl

import (
	"log"
	"net"
	"strings"
	"sync"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
	"github.com/v2fly/v2ray-core/v5/transport/internet"
)

var (
	dialBindMu    sync.RWMutex
	dialBindIface string
	dialBindUnreg func()
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
	iface := getDialBindIface()
	if iface == "" {
		return nil
	}
	return fn(iface)
}

func getDialBindIface() string {
	dialBindMu.RLock()
	defer dialBindMu.RUnlock()
	return dialBindIface
}

func setDialBindIface(iface string) {
	dialBindMu.Lock()
	dialBindIface = strings.TrimSpace(iface)
	dialBindMu.Unlock()
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
// auto 模式下缓存由默认网卡监视器回调刷新；dial 热路径只读缓存。
func EnableDialBind(iface string) {
	stopDialBindWatch()
	iface = strings.TrimSpace(iface)
	setDialBindIface(iface)
	if iface == "" || !BindInterfaceAuto() {
		return
	}
	if err := singtun.EnsureStandaloneMonitor(); err != nil {
		log.Printf("dial-bind watch: monitor: %v", err)
		return
	}
	unreg, err := singtun.RegisterDefaultInterfaceUpdate(onDialBindIfaceUpdate)
	if err != nil {
		log.Printf("dial-bind watch: %v", err)
		return
	}
	dialBindMu.Lock()
	dialBindUnreg = unreg
	dialBindMu.Unlock()
}

func onDialBindIfaceUpdate(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		// 短暂无默认口时保留上一跳，避免出站绑卡被清空。
		return
	}
	setDialBindIface(name)
	syncAutoBindSnap(name)
}

func stopDialBindWatch() {
	dialBindMu.Lock()
	unreg := dialBindUnreg
	dialBindUnreg = nil
	dialBindMu.Unlock()
	if unreg != nil {
		unreg()
	}
}

// DisableDialBind 关闭出站绑网卡（TUN 关闭或降级时调用）。
func DisableDialBind() {
	stopDialBindWatch()
	setDialBindIface("")
}
