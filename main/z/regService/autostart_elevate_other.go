//go:build !windows

package regService

// InstallAutoStartWithUAC 非 Windows 直接走 InstallAutoStart。
func InstallAutoStartWithUAC() error {
	return InstallAutoStart()
}

// UninstallAutoStartWithUAC 非 Windows 直接走 UninstallAutoStart。
func UninstallAutoStartWithUAC() error {
	return UninstallAutoStart()
}
