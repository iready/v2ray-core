package rocket

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
)

const tunRecoveryInterval = 30 * time.Second

// The accepted configuration is owned by the lifecycle entry, not captured by this loop.
func (x *RS) RunTUNRecoveryLoop(ctx context.Context) {
	ticker := time.NewTicker(tunRecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := x.tryRecoverTUN(ctx); err != nil {
				log.Printf("phase=tun_recovery result=failed err=%v", err)
			}
		}
	}
}

func (x *RS) activateTUN(owner string) (err error) {
	at := time.Now()
	defer func() { log.Printf("phase=tun_activate owner=%q elapsed=%s err=%v", owner, time.Since(at), err) }()
	if err = tunctl.Default().ApplyRoutes(); err != nil {
		return err
	}
	if err = tunctl.Default().ApplyDNS(); err != nil {
		return err
	}
	x.setTunRuntime(owner, true, "")
	return nil
}

// startTUNOwner is shared by initial startup and owner-only recovery.
// A successful fallback means the proxy instance runs, not that TUN is active.
func (x *RS) startTUNOwner(owner *ServerInstance, servers map[string]*ServerInstance) error {
	err := x.prepareTUNEnvironment(owner.Key, owner.RawJSON, collectServerRawJSONs(servers), x.tunProfile())
	if err == nil {
		iface, _ := x.tunBindRuntimeLocked()
		err = applyOutboundBindToServer(owner, iface)
	}
	if err == nil {
		err = x.StartServer(owner)
	}
	if err == nil {
		err = x.activateTUN(owner.Key)
	}
	if err == nil {
		return nil
	}
	if fallbackErr := x.startOwnerWithoutTUN(owner, err); fallbackErr != nil {
		return fmt.Errorf("TUN: %w; fallback: %v", err, fallbackErr)
	}
	return nil
}

func (x *RS) startOwnerWithoutTUN(owner *ServerInstance, cause error) error {
	_ = tunctl.Default().SuspendRoutes()
	if owner == nil {
		return fmt.Errorf("missing TUN owner")
	}
	if err := x.StopServer(owner); err != nil {
		_ = tunctl.Default().Cleanup()
		x.setTunRuntime(owner.Key, false, err.Error())
		return err
	}
	_ = tunctl.Default().Cleanup()
	work := *owner
	if err := stripTUNFromServer(&work); err != nil {
		return err
	}
	x.serversMu.Lock()
	*owner = work
	x.serversMu.Unlock()
	x.setTunRuntime(owner.Key, false, cause.Error())
	log.Printf("phase=tun_degrade owner=%q cause=%v", owner.Key, cause)
	return x.StartServer(owner)
}

func (x *RS) tryRecoverTUN(ctx context.Context) error {
	// Skip rather than queue behind a reload; the next tick uses the latest accepted config.
	if !x.lifecycleMu.TryLock() {
		return nil
	}
	defer x.lifecycleMu.Unlock()
	if x.stopping || ctx.Err() != nil || !x.tunProfile().Use {
		return nil
	}
	_, active, _ := x.tunRuntimeLocked()
	if active || x.latestConfig == nil {
		return nil
	}
	if err := tunctl.WaitReady(ctx); err != nil {
		return err
	}
	servers, err := x.ToV2Config(x.latestConfig)
	if err != nil {
		return err
	}
	ownerKey := findTunOwnerKey(servers)
	owner := servers[ownerKey]
	if owner == nil {
		return nil
	}
	if !tunctl.HasValidProxyListRoutes(owner.RawJSON) {
		return fmt.Errorf("proxy_list empty")
	}
	defer x.stateChanged()
	at := time.Now()
	log.Printf("phase=tun_recovery owner=%q version=%d result=begin", ownerKey, x.latestConfig.Version)
	old := x.snapshotServers()[ownerKey]
	_ = tunctl.Default().SuspendRoutes()
	if old != nil {
		if err := x.StopServer(old); err != nil {
			return err
		}
	}
	x.serversMu.Lock()
	delete(x.Servers, ownerKey)
	x.serversMu.Unlock()
	if err = x.startTUNOwner(owner, servers); err != nil {
		return err
	}
	x.serversMu.Lock()
	x.Servers[ownerKey] = owner
	x.serversMu.Unlock()
	log.Printf("phase=tun_recovery owner=%q elapsed=%s err=%v other_instances=untouched", ownerKey, time.Since(at), err)
	return err
}
