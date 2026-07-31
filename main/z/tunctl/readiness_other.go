//go:build !darwin && !windows

package tunctl

import "context"

// WaitReady 非 macOS/Windows 无 Privileged Helper / UAC 预检，直接返回。
func WaitReady(ctx context.Context) error {
	return nil
}
