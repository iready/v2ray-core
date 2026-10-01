//go:build !darwin && !windows

package tunctl

func RepairStaleSystemDNS() error { return nil }
