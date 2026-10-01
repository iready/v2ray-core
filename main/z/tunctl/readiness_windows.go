//go:build windows

package tunctl

import (
	"context"
	"fmt"
)

// WaitReady 要求进程已提权；否则 TUN 启动必失败且恢复循环会空转。
func WaitReady(ctx context.Context) error {
	_ = ctx
	if IsElevated() {
		return nil
	}
	return fmt.Errorf("TUN requires Administrator (re-run rocket.exe elevated)")
}
