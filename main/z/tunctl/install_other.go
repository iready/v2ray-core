//go:build !darwin

package tunctl

import "fmt"

func InstallHelper(helperBinary string) error {
	return fmt.Errorf("privileged helper install is only supported on macOS")
}

func UninstallHelper() error {
	return fmt.Errorf("privileged helper uninstall is only supported on macOS")
}

func HelperBinaryPath() string { return "v2ray-helper" }
