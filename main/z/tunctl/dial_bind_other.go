//go:build !darwin && !linux && !windows

package tunctl

func bindOutboundToInterface(network, address string, fd uintptr, device string) error {
	_ = network
	_ = address
	_ = fd
	_ = device
	return nil
}
