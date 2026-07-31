//go:build darwin

package tunctl

import (
	"errors"
	"net"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
	"golang.org/x/sys/unix"
)

var errNoBindInterface = errors.New("no outbound bind interface")

func bindOutboundToInterface(network, address string, fd uintptr, device string) error {
	_ = address
	name := strings.TrimSpace(device)
	if name == "" {
		name = strings.TrimSpace(singtun.CurrentInterfaceName())
	}
	if name == "" {
		return errNoBindInterface
	}
	ensureDirectBypassRoute(network, address)
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return err
	}
	if strings.HasSuffix(network, "6") {
		if err := unix.SetsockoptInt(int(fd), unix.IPPROTO_IPV6, unix.IPV6_BOUND_IF, iface.Index); err != nil {
			return err
		}
		return nil
	}
	if err := unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_BOUND_IF, iface.Index); err != nil {
		return err
	}
	return nil
}
