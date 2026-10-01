//go:build darwin

package tunctl

import (
	"context"
	"fmt"
	"time"
)

const helperReadyPoll = 2 * time.Second

// WaitReady 轮询直到 Privileged Helper 可响应，或快速失败（避免无 Helper 时卡 90s）。
func WaitReady(ctx context.Context) error {
	deadline := time.Now().Add(8 * time.Second)
	for {
		if Default().HelperInstalled() {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("helper not installed or not responding (open admin UI → TUN → 安装 Helper)")
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("helper not ready: %w", ctx.Err())
		case <-time.After(helperReadyPoll):
		}
	}
}
