package rocket

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
	"github.com/v2fly/v2ray-core/v5/main/z/wire"
)

// AgentHooks v2fly_wire 推送回调。
type AgentHooks struct {
	OnConfig  func(*pb.GetConfigRes)
	OnExecute func([]*pb.Execute)
}

func (rs *RS) machineName() string {
	host, _ := os.Hostname()
	if host == "" {
		return "v2fly-agent"
	}
	return host
}

// ConnectHello 建连并 hello。
// TUN 已启用时短暂 SuspendRoutes，避免控制面被 split route 拐进 utun；
// dial 结束立即 Resume，本地代理在失败退避期间继续可用。
func (rs *RS) ConnectHello(ctx context.Context) (*pb.GetConfigRes, error) {
	if err := tunctl.SuspendRoutes(); err != nil {
		log.Printf("suspend TUN routes before wire dial: %v", err)
	}
	defer func() {
		if err := tunctl.ResumeRoutes(); err != nil {
			log.Printf("resume TUN routes: %v", err)
		}
	}()

	rs.disconnectWire()
	client, err := wire.Dial(rs.Address, rs.Token, rs.Sign, rs.WireTLS)
	if err != nil {
		rs.setWireError(err)
		return nil, err
	}
	rs.wireClient = client
	client.Start()
	payload, err := client.Hello(ctx, rs.machineName())
	if err != nil {
		rs.disconnectWire()
		rs.setWireError(err)
		return nil, err
	}
	rs.setWireConnected(true)
	return payload.ToGetConfigRes(), nil
}

// Reconnect 断开并重新 hello（供本地后台触发）。
func (rs *RS) Reconnect(ctx context.Context) (*pb.GetConfigRes, error) {
	return rs.ConnectHello(ctx)
}

// ConnectHelloRetry 重试直到成功或 ctx 取消。
func (rs *RS) ConnectHelloRetry(ctx context.Context, interval time.Duration) (*pb.GetConfigRes, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cfg, err := rs.ConnectHello(ctx)
		if err == nil {
			return cfg, nil
		}
		if sleepOrDone(ctx, interval) {
			return nil, ctx.Err()
		}
	}
}

func (rs *RS) DisconnectWire() { rs.disconnectWire() }

func (rs *RS) disconnectWire() {
	if rs.wireClient != nil {
		_ = rs.wireClient.Close()
		rs.wireClient = nil
	}
	rs.setWireConnected(false)
}

func (rs *RS) reportExecuteResult(ctx context.Context, exec *pb.Execute, execErr error) {
	if rs.wireClient == nil {
		return
	}
	msg, ok := "执行成功", true
	if execErr != nil {
		msg, ok = execErr.Error(), false
	}
	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_ = rs.wireClient.UploadExecuteResult(tctx, exec.Id, msg, ok)
}

// AppliedTokenVersion 返回 agent 已成功应用的配置版本（供 ping 上报）。
func (rs *RS) AppliedTokenVersion() int32 {
	return rs.TokenVersion
}

// ReloadConfig 停实例、按新配置重启并更新 token 版本。
// 第二个返回值表示是否实际执行了重载（false 表示版本未变而跳过）。
func (rs *RS) ReloadConfig(cfg *pb.GetConfigRes) (bool, error) {
	if cfg == nil || (cfg.R != nil && cfg.R.Code != pb.Code_OK) {
		return false, nil
	}
	if cfg.Version <= rs.TokenVersion && len(rs.Servers) > 0 {
		log.Printf("配置版本未变 (applied=%d incoming=%d)，跳过热更新", rs.TokenVersion, cfg.Version)
		return false, nil
	}
	servers, err := rs.ToV2Config(cfg)
	if err != nil {
		return false, err
	}

	oldServers := rs.snapshotServers()
	oldVersion := rs.TokenVersion

	rs.StopAllServers()
	if err := rs.StartAllServers(servers, cfg.Version); err != nil {
		log.Printf("配置热更新失败，回滚旧配置: %v", err)
		if len(oldServers) > 0 {
			if rbErr := rs.StartAllServersNoReport(oldServers, oldVersion); rbErr != nil {
				log.Printf("回滚旧配置失败: %v", rbErr)
				rs.ReportAgentStatus(false, truncateReport(err.Error()+"；回滚失败: "+rbErr.Error(), maxReportSummary), servers)
			} else {
				rs.ReportAgentStatus(false, truncateReport(err.Error()+"（已回滚旧配置）", maxReportSummary), servers)
			}
		} else {
			rs.ReportAgentStatus(false, truncateReport(err.Error(), maxReportSummary), servers)
		}
		return false, err
	}
	return true, nil
}

// RunAgent 阻塞：维持 WS 长连接，断线指数退避重连。
// 本地实例与缓存配置不随 Wire 断开而停；仅 dial 窗口短暂卸 TUN 路由。
func (rs *RS) RunAgent(ctx context.Context, hooks AgentHooks) {
	backoff := 5 * time.Second
	const maxBackoff = 2 * time.Minute

	for {
		if err := ctx.Err(); err != nil {
			log.Println("agent 退出:", err)
			return
		}

		if rs.wireClient == nil {
			connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			cfg, err := rs.ConnectHello(connectCtx)
			cancel()
			if err != nil {
				log.Printf("wire 连接失败: %v", err)
				if sleepOrDone(ctx, backoff) {
					return
				}
				backoff = min(backoff*2, maxBackoff)
				continue
			}
			backoff = 5 * time.Second
			if hooks.OnConfig != nil && cfg != nil {
				hooks.OnConfig(cfg)
			}
		}

		sessionErr := rs.runWireSession(ctx, hooks)
		if ctx.Err() != nil {
			return
		}
		log.Printf("wire 断开: %v，%v 后重连", sessionErr, backoff)
		rs.disconnectWire()
		if sleepOrDone(ctx, backoff) {
			return
		}
		backoff = min(backoff*2, maxBackoff)
	}
}

func (rs *RS) runWireSession(ctx context.Context, hooks AgentHooks) error {
	client := rs.wireClient
	if client == nil {
		return fmt.Errorf("wire: not connected")
	}
	client.SetPushHandlers(
		func(p *wire.ConfigPayload) {
			if p == nil || !p.Ok {
				return
			}
			if hooks.OnConfig != nil {
				hooks.OnConfig(p.ToGetConfigRes())
			}
		},
		hooks.OnExecute,
	)
	client.StartPingLoop(ctx, rs.AppliedTokenVersion)
	return client.Wait(ctx)
}

func sleepOrDone(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(d):
		return false
	}
}
