package rocket

import (
	"context"
	"log"
	"time"

	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
)

const tunRecoveryInterval = 30 * time.Second

// RunTUNRecoveryLoop 在 TUN 因启动时未就绪而降级后，周期性尝试完整恢复。
func (x *RS) RunTUNRecoveryLoop(ctx context.Context, cfg *pb.GetConfigRes) {
	if cfg == nil {
		return
	}
	ticker := time.NewTicker(tunRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := x.tryRecoverTUN(ctx, cfg); err != nil {
				log.Printf("TUN 恢复重试: %v", err)
			}
		}
	}
}

func (x *RS) tryRecoverTUN(ctx context.Context, cfg *pb.GetConfigRes) error {
	if !x.tunProfile().Use {
		return nil
	}
	_, active, _ := x.tunRuntimeLocked()
	if active {
		return nil
	}
	if len(x.Servers) == 0 {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	err := tunctl.WaitReady(waitCtx)
	cancel()
	if err != nil {
		return err
	}
	log.Println("TUN 依赖已就绪，尝试恢复")
	return x.restartWithTUN(cfg)
}

func (x *RS) restartWithTUN(cfg *pb.GetConfigRes) error {
	servers, err := x.ToV2Config(cfg)
	if err != nil {
		return err
	}
	x.StopAllServers()
	return x.StartAllServers(servers, cfg.Version)
}
