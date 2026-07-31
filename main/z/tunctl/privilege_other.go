//go:build !windows

package tunctl

// IsElevated 非 Windows 无 UAC；真实权限由 helper/CAP_NET_ADMIN 路径负责。
func IsElevated() bool { return true }
