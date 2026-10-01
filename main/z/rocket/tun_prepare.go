package rocket

import (
	"context"
	"fmt"
	"log"
	"time"

	core "github.com/v2fly/v2ray-core/v5"
	"github.com/v2fly/v2ray-core/v5/main/z/configmerge"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
)

const tunBootWaitTimeout = 90 * time.Second

func (x *RS) prepareTUNEnvironment(ownerKey, ownerRaw string, allRaws []string, profile rvstore.TunProfile) error {
	ctx, cancel := context.WithTimeout(context.Background(), tunBootWaitTimeout)
	defer cancel()

	if err := tunctl.WaitReady(ctx); err != nil {
		return err
	}
	bindIface, err := ensureTunBindInterfaceRetry(profile, ctx)
	if err != nil {
		return err
	}
	x.setTunBindRuntime(bindIface, tunctl.BindInterfaceAuto())
	tunctl.EnableDialBind(bindIface)

	plan := tunctl.BuildPlatformPlanForOwner(profile, x.Address, ownerRaw, allRaws...)
	tunctl.SetPlatformPlan(plan)
	tunctl.ConfigureSingtunProxyList(ownerRaw)

	var prepErr error
	for attempt := 1; attempt <= 5; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		prepErr = tunctl.Default().PrepareConfigs([]string{ownerRaw})
		if prepErr == nil {
			prepErr = tunctl.Default().ApplyBypass()
		}
		if prepErr == nil {
			return nil
		}
		log.Printf("TUN 准备第 %d 次失败: %v", attempt, prepErr)
		select {
		case <-ctx.Done():
			return fmt.Errorf("prepare TUN: %w", prepErr)
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("prepare TUN: %w", prepErr)
}

func ensureTunBindInterfaceRetry(profile rvstore.TunProfile, ctx context.Context) (string, error) {
	var lastErr error
	for {
		iface, err := ensureTunBindInterface(profile)
		if err == nil {
			return iface, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("bind interface: %w (last: %v)", ctx.Err(), lastErr)
		case <-time.After(2 * time.Second):
		}
	}
}

func stripTUNFromServers(servers map[string]*ServerInstance) error {
	for key, server := range servers {
		if server == nil {
			continue
		}
		stripped, err := configmerge.RemoveTUNService(server.RawJSON)
		if err != nil {
			return fmt.Errorf("strip tun %s: %w", key, err)
		}
		stripped, err = configmerge.RemoveOutboundBind(stripped)
		if err != nil {
			return fmt.Errorf("strip outbound bind %s: %w", key, err)
		}
		loadJSON, stripErr := configmerge.StripRocketTunFieldsForLoad(stripped)
		if stripErr != nil {
			loadJSON = stripped
		}
		cfg, err := core.LoadConfig(core.FormatJSON, []byte(loadJSON))
		if err != nil {
			return fmt.Errorf("reload config %s: %w", key, err)
		}
		server.Config = cfg
		server.RawJSON = stripped
		server.IsTunOwner = false
	}
	tunctl.DisableDialBind()
	return nil
}
