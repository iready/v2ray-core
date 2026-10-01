//go:build !windows

package tunctl

func staleLoopbackDNSInterfaces() []string { return nil }
