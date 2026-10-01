//go:build !darwin && !windows

package regService

import "fmt"

func InstallAutoStart() error {
	return fmt.Errorf("login autostart is only supported on macOS/Windows")
}

func UninstallAutoStart() error {
	return fmt.Errorf("login autostart is only supported on macOS/Windows")
}

func AutoStartInstalled() bool { return false }

func RunAutoStart() error {
	return fmt.Errorf("elevated autostart run is only supported on Windows")
}
