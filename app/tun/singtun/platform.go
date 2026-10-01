package singtun

import (
	"net/netip"
	"sync"
)

// Platform holds runtime TUN options injected by main/z/tunctl before v2ray starts.
type Platform struct {
	FileDescriptor        int
	InterfaceName         string
	AutoRoute             bool
	ExternalConfiguration bool
	RouteExcludeAddresses []netip.Prefix
	BypassHosts           []string
}

var (
	platformMu sync.Mutex
	platform   Platform
)

func SetPlatform(cfg Platform) {
	platformMu.Lock()
	platform = cfg
	platformMu.Unlock()
}

func ResetPlatform() {
	platformMu.Lock()
	platform = Platform{}
	platformMu.Unlock()
}

func takePlatform() Platform {
	platformMu.Lock()
	cfg := platform
	platform = Platform{}
	platformMu.Unlock()
	return cfg
}

func SetFileDescriptor(fd int) {
	platformMu.Lock()
	platform.FileDescriptor = fd
	platformMu.Unlock()
}

func SetInterfaceName(name string) {
	platformMu.Lock()
	platform.InterfaceName = name
	platformMu.Unlock()
}

func SetAutoRoute(enabled bool) {
	platformMu.Lock()
	platform.AutoRoute = enabled
	platformMu.Unlock()
}

func SetExternalConfiguration(enabled bool) {
	platformMu.Lock()
	platform.ExternalConfiguration = enabled
	platformMu.Unlock()
}

func SetRouteExcludeAddresses(prefixes []netip.Prefix) {
	platformMu.Lock()
	platform.RouteExcludeAddresses = append([]netip.Prefix(nil), prefixes...)
	platformMu.Unlock()
}

func SetBypassHosts(hosts []string) {
	platformMu.Lock()
	platform.BypassHosts = append([]string(nil), hosts...)
	platformMu.Unlock()
}
