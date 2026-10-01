//go:build windows

package tunctl

import "golang.org/x/sys/windows"

// IsElevated 当前进程是否已提权（TUN/路由/DNS 必需）。
func IsElevated() bool {
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return false
	}
	defer tok.Close()
	return tok.IsElevated()
}
