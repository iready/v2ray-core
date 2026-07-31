//go:build linux

package tunctl

import (
	"golang.org/x/sys/unix"
)

func bindOutboundToInterface(network, address string, fd uintptr, device string) error {
	_ = network
	_ = address
	return unix.BindToDevice(int(fd), device)
}
