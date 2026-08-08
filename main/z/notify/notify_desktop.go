//go:build windows || darwin

package notify

func desktopEnabled() bool { return true }
