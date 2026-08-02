package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	goruntime "runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/v2fly/v2ray-core/v5/main/z/localadmin"
	"github.com/v2fly/v2ray-core/v5/main/z/localconfig"
	"github.com/v2fly/v2ray-core/v5/main/z/mitmctl"
	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/regService"
	"github.com/v2fly/v2ray-core/v5/main/z/rocket"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
	"github.com/v2fly/v2ray-core/v5/main/z/wire"
)

type localConfigStore struct {
	*localconfig.Manager
}

func (s *localConfigStore) Has() bool { return s.Manager.HasConfig() }

func (s *localConfigStore) Load() (*pb.SaveConf, error) {
	return s.Manager.LoadConfig()
}

func (s *localConfigStore) Save(c *pb.GetConfigRes, token string) {
	if err := s.Manager.SaveConfig(c, token); err != nil {
		log.Printf("保存本地配置失败: %v", err)
	}
}

type runtime struct {
	store      *localConfigStore
	adminStore *localadmin.Store
	agentCfg   localadmin.AgentConfig
	adminPort  int
	rs         *rocket.RS
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.Mutex
	agentDone  chan struct{}
}

func main() {
	store := &localConfigStore{Manager: localconfig.NewManager()}
	adminStore := localadmin.NewStore()

	regToken := flag.String("reg", "", "注册模式 token（一次性安装）")
	addrFlag := flag.String("addr", "", "Rocket WebSocket 地址，如 127.0.0.1:49915/rpc 或 ws://host/rpc")
	adminPort := flag.Int("admin-port", 19527, "本地配置后台端口")
	adminMode := flag.String("admin", "on", "本地后台: on | off")
	installHelper := flag.Bool("install-helper", false, "安装 macOS Privileged Helper（需管理员授权）")
	uninstallHelper := flag.Bool("uninstall-helper", false, "拆卸 macOS Privileged Helper（需管理员授权）")
	installAutostart := flag.Bool("install-autostart", false, "注册登录自启（macOS LaunchAgent / Windows 提权计划任务）")
	uninstallAutostart := flag.Bool("uninstall-autostart", false, "取消登录自启")
	dumpConfig := flag.Bool("dump-config", false, "打印服务端下发的 v2fly 配置后退出")
	dumpConfigView := flag.String("dump-config-view", "all", "与 -dump-config 联用：all | merged | raw | <实例key> | <key>:merged")
	flag.Parse()

	if *dumpConfig {
		if err := runDumpConfig(adminStore, *dumpConfigView); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *installHelper {
		if err := tunctl.InstallHelper(tunctl.HelperBinaryPath()); err != nil {
			log.Fatal(err)
		}
		log.Printf("Helper 已安装")
		return
	}

	if *uninstallHelper {
		if err := tunctl.UninstallHelper(); err != nil {
			log.Fatal(err)
		}
		log.Printf("Helper 已拆卸")
		return
	}

	if *installAutostart {
		if err := regService.InstallAutoStart(); err != nil {
			log.Fatal(err)
		}
		log.Printf("已注册登录自启（Windows 为提权计划任务，创建时需管理员）")
		return
	}

	if *uninstallAutostart {
		if err := regService.UninstallAutoStart(); err != nil {
			log.Fatal(err)
		}
		log.Printf("已取消登录自启")
		return
	}

	if err := tunctl.RepairStaleSystemDNS(); err != nil {
		log.Printf("修复残留 TUN DNS: %v", err)
	}

	// Windows：未提权但已装计划任务 → 经任务以最高权限拉起自己后退出。
	if goruntime.GOOS == "windows" && !tunctl.IsElevated() && regService.AutoStartInstalled() {
		if err := regService.RunAutoStart(); err != nil {
			log.Printf("提权自启任务运行失败: %v（将以普通权限继续，TUN 可能不可用）", err)
		} else {
			log.Printf("已通过计划任务提权启动，本进程退出")
			return
		}
	}

	agentCfg, err := adminStore.Load()
	if err != nil {
		log.Fatalf("读取 agent.json: %v", err)
	}
	agentCfg = applyAddrFlag(agentCfg, *addrFlag)
	if file, err := adminStore.RVStore().Load(); err == nil {
		tunctl.SetUserDisabled(!file.Tun.Use)
	}

	registered := false
	if *regToken != "" {
		if err := runRegistration(store, adminStore, agentCfg, *regToken); err != nil {
			log.Fatal(err)
		}
		if !regService.InContainer() {
			return // 宿主机：regService 已由 systemd 拉起新进程
		}
		registered = true
		agentCfg, err = adminStore.Load()
		if err != nil {
			log.Fatalf("读取 agent.json: %v", err)
		}
		agentCfg = applyAddrFlag(agentCfg, *addrFlag)
	}

	if !registered {
		_, err := adminStore.RVStore().Load()
		if err != nil {
			log.Fatalf("读取 agent.json: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rt := &runtime{
		store:      store,
		adminStore: adminStore,
		agentCfg:   agentCfg,
		adminPort:  *adminPort,
		ctx:        ctx,
		cancel:     cancel,
	}

	setupSignal(rt)

	if *adminMode != "off" {
		port, ln, err := localadmin.AllocateAdminPort("127.0.0.1", *adminPort, 5)
		if err != nil {
			if file, loadErr := adminStore.RVStore().Load(); loadErr == nil {
				localadmin.KillStaleProcess(file.Runtime.PID)
				port, ln, err = localadmin.AllocateAdminPort("127.0.0.1", *adminPort, 5)
			}
		}
		if err != nil {
			log.Fatalf("绑定本地后台端口失败: %v", err)
		}
		rt.adminPort = port
		if err := adminStore.SaveRuntime(rvstore.NewRuntimeRecord(os.Getpid(), port)); err != nil {
			log.Fatalf("写入 runtime 失败: %v", err)
		}
		srv := localadmin.NewServer(ln, port, adminStore, localadmin.Hooks{
			GetRS:         rt.getRS,
			GetConfig:     rt.getAgentConfig,
			OnSave:        rt.onConfigSaved,
			OnReconnect:   rt.onReconnect,
			OnRebootstrap: rt.rebootstrap,
		})
		go func() {
			if err := srv.Serve(); err != nil {
				log.Printf("本地后台退出: %v", err)
			}
		}()
		if file, err := adminStore.RVStore().Load(); err == nil {
			if err := mitmctl.Default().Apply(file.Mitm); err != nil {
				log.Printf("MITM 启动失败: %v", err)
			}
		}
	}

	if agentCfg.IsComplete() {
		if err := rt.startAgent(); err != nil {
			log.Printf("agent 启动失败（可在本地后台修改配置）: %v", err)
		}
	} else {
		log.Printf("连接未配置，请打开 http://127.0.0.1:%d 完成设置", rt.adminPort)
	}
	log.Printf("rocket 运行中 · 本地后台 http://127.0.0.1:%d · 按 Ctrl+C 退出", rt.adminPort)

	<-ctx.Done()
}

func (rt *runtime) getRS() *rocket.RS {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.rs
}

func (rt *runtime) getAgentConfig() localadmin.AgentConfig {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.agentCfg
}

func (rt *runtime) tunProfileLoader() rvstore.TunProfile {
	file, err := rt.adminStore.RVStore().Load()
	if err != nil {
		return rvstore.DefaultTunProfile()
	}
	return file.Tun
}

func (rt *runtime) logProfileLoader() rvstore.LogProfile {
	file, err := rt.adminStore.RVStore().Load()
	if err != nil {
		return rvstore.DefaultLogProfile()
	}
	return file.LogProfile
}

func (rt *runtime) newRS(cfg localadmin.AgentConfig) (*rocket.RS, error) {
	tls, err := cfg.WireTLS()
	if err != nil {
		return nil, err
	}
	return &rocket.RS{
		Address:        cfg.ResolvedAddr(),
		Token:          cfg.Token,
		Sign:           cfg.Sign,
		WireTLS:        tls,
		Servers:        make(map[string]*rocket.ServerInstance),
		GlobalCancel:   rt.cancel,
		TunProfileFunc: rt.tunProfileLoader,
		LogProfileFunc: rt.logProfileLoader,
	}, nil
}

func (rt *runtime) onConfigSaved(cfg localadmin.AgentConfig) error {
	rt.mu.Lock()
	rt.agentCfg = cfg
	rs := rt.rs
	running := rt.agentDone != nil
	rt.mu.Unlock()

	tls, err := cfg.WireTLS()
	if err != nil {
		return err
	}
	if rs == nil {
		newRS, err := rt.newRS(cfg)
		if err != nil {
			return err
		}
		rt.mu.Lock()
		rt.rs = newRS
		rt.mu.Unlock()
	} else {
		rs.ApplyConnection(cfg.ResolvedAddr(), cfg.Token, cfg.Sign, tls)
		rs.TunProfileFunc = rt.tunProfileLoader
		rs.LogProfileFunc = rt.logProfileLoader
	}

	if !cfg.IsComplete() {
		return nil
	}
	if !running {
		return rt.startAgent()
	}
	return rt.rebootstrap()
}

func (rt *runtime) onReconnect() error {
	rt.mu.Lock()
	rs := rt.rs
	rt.mu.Unlock()
	if rs == nil {
		return fmt.Errorf("agent 未配置")
	}
	ctx, cancel := context.WithTimeout(rt.ctx, 30*time.Second)
	defer cancel()
	_, err := rs.Reconnect(ctx)
	return err
}

func (rt *runtime) startAgent() error {
	rt.mu.Lock()
	if rt.agentDone != nil {
		rt.mu.Unlock()
		return nil
	}
	cfg := rt.agentCfg
	if rt.rs == nil {
		rs, err := rt.newRS(cfg)
		if err != nil {
			rt.mu.Unlock()
			return err
		}
		rt.rs = rs
	}
	rs := rt.rs
	rt.agentDone = make(chan struct{})
	rt.mu.Unlock()

	go func() {
		defer close(rt.agentDone)
		if err := rt.runAgentLoop(rs); err != nil {
			log.Printf("agent 退出: %v", err)
		}
		rt.mu.Lock()
		rt.agentDone = nil
		rt.mu.Unlock()
	}()
	return nil
}

func (rt *runtime) runAgentLoop(rs *rocket.RS) error {
	serverCfg, err := bootstrapConfig(rt.ctx, rs, rt.store)
	if err != nil {
		return err
	}
	if err := startServersWithRetry(rt.ctx, rs, serverCfg); err != nil {
		return err
	}
	go rs.RunTUNRecoveryLoop(rt.ctx, serverCfg)
	rs.RunAgent(rt.ctx, rocket.AgentHooks{
		OnConfig: func(c *pb.GetConfigRes) {
			reloaded, err := rs.ReloadConfig(c)
			if err != nil {
				log.Printf("配置热更新失败: %v", err)
				return
			}
			if !reloaded {
				return
			}
			rt.store.Save(c, rs.Token)
			log.Println("配置热更新完成")
		},
		OnExecute: func(cmds []*pb.Execute) {
			if len(cmds) == 0 {
				return
			}
			log.Printf("收到 %d 条远程命令", len(cmds))
			rs.Execute(rt.ctx, cmds)
		},
	})
	return nil
}

func (rt *runtime) rebootstrap() error {
	rt.mu.Lock()
	rs := rt.rs
	rt.mu.Unlock()
	if rs == nil {
		return fmt.Errorf("agent 未初始化")
	}

	ctx, cancel := context.WithTimeout(rt.ctx, 45*time.Second)
	defer cancel()

	// 先 hello（ConnectHello 内短暂 SuspendRoutes），旧实例继续代理；
	// 拿到配置后再停旧启新，避免「等配置时整机像断网」。
	serverCfg, err := rs.Reconnect(ctx)
	if err != nil {
		return err
	}
	rt.store.Save(serverCfg, rs.Token)
	servers, err := rs.ToV2Config(serverCfg)
	if err != nil {
		return err
	}
	rs.StopAllServers()
	return rs.StartAllServers(servers, serverCfg.Version)
}

func isValidConfig(c *pb.GetConfigRes) bool {
	if c == nil || (c.R != nil && c.R.Code != pb.Code_OK) || len(c.Config) == 0 {
		return false
	}
	for _, item := range c.Config {
		if item.Key == "" || item.Config == "" {
			return false
		}
	}
	return true
}

func bootstrapConfig(ctx context.Context, rs *rocket.RS, store *localConfigStore) (*pb.GetConfigRes, error) {
	helloCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	remote, err := rs.ConnectHello(helloCtx)
	cancel()
	if err == nil && isValidConfig(remote) {
		store.Save(remote, rs.Token)
		return remote, nil
	}
	if err != nil {
		log.Printf("wire hello 失败: %v", err)
		rs.DisconnectWire()
	}
	if store.Has() {
		local, loadErr := store.Load()
		if loadErr == nil && isValidConfig(local.Config) {
			log.Printf("使用本地缓存配置 (token_version=%d)", local.Config.Version)
			return local.Config, nil
		}
	}
	if err != nil {
		remote, err = rs.ConnectHelloRetry(ctx, 15*time.Second)
		if err != nil {
			return nil, err
		}
		store.Save(remote, rs.Token)
		return remote, nil
	}
	return nil, fmt.Errorf("无可用配置")
}

func startServers(rs *rocket.RS, c *pb.GetConfigRes) error {
	servers, err := rs.ToV2Config(c)
	if err != nil {
		return err
	}
	return rs.StartAllServers(servers, c.Version)
}

func startServersWithRetry(ctx context.Context, rs *rocket.RS, cfg *pb.GetConfigRes) error {
	const total = 2 * time.Minute
	deadline := time.Now().Add(total)
	backoff := 3 * time.Second
	var lastErr error
	for {
		lastErr = startServers(rs, cfg)
		if lastErr == nil {
			return nil
		}
		if !isRetryableStartError(lastErr) {
			return lastErr
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		log.Printf("实例启动失败，%v 后重试: %v", backoff, lastErr)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 15*time.Second {
			backoff += 2 * time.Second
		}
	}
}

func isRetryableStartError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "proxy_list empty"):
		return false
	case strings.Contains(msg, "no valid server"):
		return false
	case strings.Contains(msg, "no config items"):
		return false
	default:
		return true
	}
}

func applyAddrFlag(cfg localadmin.AgentConfig, addr string) localadmin.AgentConfig {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return cfg
	}
	url := wire.NormalizeWSURL(addr)
	if cfg.Active == "" {
		cfg.Active = "cli"
	}
	for i, p := range cfg.Presets {
		if p.Name == cfg.Active {
			cfg.Presets[i].URL = url
			return cfg
		}
	}
	cfg.Presets = append(cfg.Presets, rvstore.AddressPreset{Name: cfg.Active, URL: url})
	return cfg
}

func runRegistration(store *localConfigStore, adminStore *localadmin.Store, agentCfg localadmin.AgentConfig, regToken string) error {
	log.Printf("注册模式 token=%s", regToken)
	addr := agentCfg.ResolvedAddr()
	if addr == "" {
		agentCfg = localadmin.DefaultConfig()
		addr = agentCfg.ResolvedAddr()
	}
	tls, err := agentCfg.WireTLS()
	if err != nil {
		return err
	}
	rs := &rocket.RS{
		Address: addr,
		Token:   regToken,
		Sign:    agentCfg.Sign,
		WireTLS: tls,
		Servers: make(map[string]*rocket.ServerInstance),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	serverCfg, err := rs.ConnectHello(ctx)
	if err != nil {
		regService.WriteToFile()
		return fmt.Errorf("获取配置: %w", err)
	}
	if agentCfg.ResolvedAddr() == "" {
		agentCfg = localadmin.DefaultConfig()
	}
	agentCfg.Token = regToken
	if err := adminStore.Save(agentCfg); err != nil {
		return fmt.Errorf("保存连接配置: %w", err)
	}
	store.Save(serverCfg, regToken)
	regService.WriteToFile()
	log.Println("注册完成")
	return nil
}

func runDumpConfig(adminStore *localadmin.Store, arg string) error {
	cached, err := adminStore.RVStore().LoadConfigCache()
	if err != nil {
		return fmt.Errorf("读取缓存配置: %w", err)
	}
	if cached == nil || cached.Config == nil {
		return fmt.Errorf("本地无服务端配置缓存，请先启动 rocket 并成功连接")
	}
	file, err := adminStore.RVStore().Load()
	if err != nil {
		return fmt.Errorf("读取 agent.json: %w", err)
	}

	arg = strings.TrimSpace(arg)
	includeRaw := true
	includeMerged := true
	mergedOnly := false
	keyFilter := ""

	switch strings.ToLower(arg) {
	case "", "all":
		// 默认：全部实例，raw + merged
	case "merged":
		includeRaw = false
		includeMerged = true
		mergedOnly = true
	case "raw":
		includeMerged = false
	default:
		if strings.HasSuffix(strings.ToLower(arg), ":merged") {
			keyFilter = strings.TrimSuffix(arg, ":merged")
			keyFilter = strings.TrimSuffix(keyFilter, ":MERGED")
			includeRaw = false
			includeMerged = true
			mergedOnly = true
		} else if strings.HasSuffix(strings.ToLower(arg), ":raw") {
			keyFilter = arg[:len(arg)-4]
			includeMerged = false
		} else {
			keyFilter = arg
			includeMerged = false
		}
	}

	dump, err := rocket.BuildServerConfigDump(cached.Config, file.Tun, file.LogProfile, includeRaw, includeMerged, keyFilter, mergedOnly)
	if err != nil {
		return err
	}
	_, _ = fmt.Print(rocket.FormatServerConfigDump(dump, mergedOnly))
	return nil
}

func setupSignal(rt *runtime) {
	signal.Ignore(syscall.SIGHUP)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-ch
		log.Printf("收到 %v，关闭中...", sig)
		rt.cancel()
		_ = mitmctl.Default().Stop()
		rt.mu.Lock()
		if rt.rs != nil {
			rt.rs.StopAllServers()
			rt.rs.DisconnectWire()
		}
		rt.mu.Unlock()
		// 不在退出路径写 agent.json：部署会 TERM 后很快 KILL，
		// 旧 WriteFile 截断窗口会留下空文件；残留 runtime.pid 由 KillStaleProcess 判断存活。
		os.Exit(0)
	}()
}
