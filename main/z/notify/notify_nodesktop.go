//go:build !windows && !darwin

package notify

func desktopEnabled() bool { return false }

func show(title, body string) error { return nil }
