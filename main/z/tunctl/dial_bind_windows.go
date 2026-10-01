//go:build windows

package tunctl

import (
	"encoding/binary"
	"strings"
	"unsafe"

	"github.com/v2fly/v2ray-core/v5/transport/internet"
	"golang.org/x/sys/windows"
)

const (
	ipUnicastIf   = 31
	ipv6UnicastIf = 31
)

func clearBindIfIndex() { internet.ForgetInterfaceIndex() }

func bindOutboundToInterface(network, address string, fd uintptr, device string) error {
	_ = address
	ifIndex, err := internet.InterfaceIndex(device)
	if err != nil {
		return err
	}
	handle := windows.Handle(fd)
	if strings.HasSuffix(network, "6") {
		return windows.SetsockoptInt(handle, windows.IPPROTO_IPV6, ipv6UnicastIf, ifIndex)
	}
	// Windows IPv4 IP_UNICAST_IF：network-order 接口 index（与 sockopt_windows 一致）。
	var be [4]byte
	binary.BigEndian.PutUint32(be[:], uint32(ifIndex))
	idx := int(*(*uint32)(unsafe.Pointer(&be[0])))
	return windows.SetsockoptInt(handle, windows.IPPROTO_IP, ipUnicastIf, idx)
}
