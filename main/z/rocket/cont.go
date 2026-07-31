package rocket

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	core "github.com/v2fly/v2ray-core/v5"
	"github.com/v2fly/v2ray-core/v5/main/z/configmerge"
	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
	"github.com/v2fly/v2ray-core/v5/main/z/wire"
)

// ServerInstance 表示单个服务器实例
type ServerInstance struct {
	Key           string             // 服务器key
	Config        *core.Config       // V2Ray配置
	RawJSON       string             // 原始 v2fly JSON（Rocket 下发）
	Version       int32              // 配置版本
	Status        pb.ServerStatus    // 服务器状态
	IsTunOwner    bool               // 是否为唯一 TUN 拥有者
	CancelFunc    context.CancelFunc // 取消函数
	V2flyServer   *core.Instance     // V2Ray实例
	LastHeartbeat time.Time          // 最后心跳/启动时间
	LastError     string             // 最近一次启动失败原因
}

// RuntimeStatus 供本地后台读取的运行时快照。
type RuntimeStatus struct {
	WireConnected     bool
	ConnectedSince    time.Time
	LastWireError     string
	StartedAt         time.Time
	TunOwnerKey       string
	TunActive         bool
	TunDegradedReason string
	TunBindInterface  string
	TunBindAuto       bool
}

// RS 管理多个服务器实例
type RS struct {
	Address      string
	Token        string
	Sign         string
	TokenVersion int32 // 当前token版本号
	WireTLS      *wire.TLSConfig
	wireClient   *wire.Client
	Servers      map[string]*ServerInstance // key -> 服务器实例映射
	GlobalCancel context.CancelFunc         // 全局取消函数

	statusMu       sync.RWMutex
	wireConnected  bool
	connectedSince time.Time
	lastWireError  string
	startedAt      time.Time

	TunProfileFunc func() rvstore.TunProfile
	LogProfileFunc func() rvstore.LogProfile

	tunMu             sync.RWMutex
	tunOwnerKey       string
	tunActive         bool
	tunDegradedReason string
	tunBindInterface  string
	tunBindAuto       bool
}

func (x *RS) Status() RuntimeStatus {
	x.statusMu.RLock()
	defer x.statusMu.RUnlock()
	owner, active, degraded := x.tunRuntimeLocked()
	bindIface, bindAuto := x.tunBindRuntimeLocked()
	return RuntimeStatus{
		WireConnected:     x.wireConnected,
		ConnectedSince:    x.connectedSince,
		LastWireError:     x.lastWireError,
		StartedAt:         x.startedAt,
		TunOwnerKey:       owner,
		TunActive:         active,
		TunDegradedReason: degraded,
		TunBindInterface:  bindIface,
		TunBindAuto:       bindAuto,
	}
}

func (x *RS) tunRuntimeLocked() (ownerKey string, active bool, degraded string) {
	x.tunMu.RLock()
	defer x.tunMu.RUnlock()
	return x.tunOwnerKey, x.tunActive, x.tunDegradedReason
}

func (x *RS) tunBindRuntimeLocked() (iface string, auto bool) {
	x.tunMu.RLock()
	defer x.tunMu.RUnlock()
	return x.tunBindInterface, x.tunBindAuto
}

func (x *RS) setTunBindRuntime(iface string, auto bool) {
	x.tunMu.Lock()
	defer x.tunMu.Unlock()
	x.tunBindInterface = iface
	x.tunBindAuto = auto
}

func (x *RS) setTunRuntime(ownerKey string, active bool, degraded string) {
	x.tunMu.Lock()
	defer x.tunMu.Unlock()
	x.tunOwnerKey = ownerKey
	x.tunActive = active
	x.tunDegradedReason = degraded
}

func (x *RS) setWireConnected(ok bool) {
	x.statusMu.Lock()
	defer x.statusMu.Unlock()
	x.wireConnected = ok
	if ok {
		x.connectedSince = time.Now()
		x.lastWireError = ""
	} else {
		x.connectedSince = time.Time{}
	}
}

func (x *RS) setWireError(err error) {
	if err == nil {
		return
	}
	x.statusMu.Lock()
	defer x.statusMu.Unlock()
	x.lastWireError = err.Error()
	x.wireConnected = false
}

func (x *RS) markStarted() {
	x.statusMu.Lock()
	defer x.statusMu.Unlock()
	if x.startedAt.IsZero() {
		x.startedAt = time.Now()
	}
}

// ApplyConnection 更新连接参数（本地后台保存后调用）。
func (x *RS) ApplyConnection(addr, token, sign string, tls *wire.TLSConfig) {
	x.Address = addr
	x.Token = token
	x.Sign = sign
	x.WireTLS = tls
}

// IsWireConnected 当前 wire 是否在线。
func (x *RS) IsWireConnected() bool {
	x.statusMu.RLock()
	defer x.statusMu.RUnlock()
	return x.wireConnected && x.wireClient != nil
}
func (x *RS) tunProfile() rvstore.TunProfile {
	if x.TunProfileFunc != nil {
		return x.TunProfileFunc()
	}
	return rvstore.DefaultTunProfile()
}

func (x *RS) logProfile() rvstore.LogProfile {
	if x.LogProfileFunc != nil {
		return x.LogProfileFunc()
	}
	return rvstore.DefaultLogProfile()
}

func (x *RS) mergeServerJSONForItem(raw string, isOwner bool) string {
	profile := x.tunProfile()
	var merged string
	var err error
	if isOwner {
		merged, err = configmerge.ApplyTunProfile(raw, profile)
	} else {
		merged, err = configmerge.ApplyTunProfileNonOwner(raw, profile)
	}
	if err != nil {
		log.Printf("merge TUN profile: %v", err)
		merged = raw
	}
	if isOwner && profile.Use {
		var policyErr error
		merged, policyErr = configmerge.ApplyTunOwnerPolicy(merged)
		if policyErr != nil {
			log.Printf("merge TUN owner policy: %v", policyErr)
		}
		merged, policyErr = configmerge.ApplyTunOwnerDNSPolicy(merged)
		if policyErr != nil {
			log.Printf("merge TUN owner DNS policy: %v", policyErr)
		}
	}
	merged, err = configmerge.ApplyLogProfile(merged, x.logProfile())
	if err != nil {
		log.Printf("merge log profile: %v", err)
		return merged
	}
	return merged
}

func (x *RS) mergeServerJSON(raw string) string {
	return x.mergeServerJSONForItem(raw, true)
}

func (x *RS) ToV2Config(c *pb.GetConfigRes) (map[string]*ServerInstance, error) {
	// 检查响应结果
	if c.R != nil && c.R.Code != pb.Code_OK {
		return nil, fmt.Errorf("server error: %s", c.R.Msg)
	}

	if len(c.Config) == 0 {
		return nil, fmt.Errorf("no config items received")
	}

	// 初始化服务器映射
	servers := make(map[string]*ServerInstance)

	// 为每个ConfigItem创建ServerInstance
	profile := x.tunProfile()
	tunctl.SetUserDisabled(!profile.Use)

	tunOwnerKey, _ := ResolveTunOwner(profile, c.Config)
	x.setTunRuntime(tunOwnerKey, false, "")
	if profile.Use {
		iface, err := ensureTunBindInterface(profile)
		if err != nil {
			log.Printf("TUN bind interface 未就绪（将在启动时重试）: %v", err)
			if manual := strings.TrimSpace(profile.BindInterface); manual != "" {
				x.setTunBindRuntime(manual, false)
			}
		} else {
			x.setTunBindRuntime(iface, tunctl.BindInterfaceAuto())
		}
	} else {
		tunctl.ResetBindInterface()
		tunctl.DisableDialBind()
		x.setTunBindRuntime("", false)
	}

	for _, configItem := range c.Config {
		if configItem == nil {
			continue
		}
		isOwner := tunOwnerKey != "" && configItem.Key == tunOwnerKey
		rawJSON := x.mergeServerJSONForItem(configItem.Config, isOwner)
		loadJSON, stripErr := configmerge.StripRocketTunFieldsForLoad(rawJSON)
		if stripErr != nil {
			log.Printf("strip rocket tun fields for key %s: %v", configItem.Key, stripErr)
			loadJSON = rawJSON
		}
		if !isOwner && tunctl.HasTUNService(loadJSON) {
			stripped, err := configmerge.RemoveTUNService(loadJSON)
			if err != nil {
				log.Printf("remove duplicate tun for key %s: %v", configItem.Key, err)
			} else {
				loadJSON = stripped
				if tunOwnerKey != "" {
					log.Printf("TUN 由实例 %s 独占，跳过 %s 的 services.tun", tunOwnerKey, configItem.Key)
				}
			}
		}
		// 解析V2Ray配置（RawJSON 保留 route_mode 等扩展供 tunctl 使用）
		v2Config := &core.Config{}
		var err error
		v2Config, err = core.LoadConfig(core.FormatJSON, []byte(loadJSON))
		if err != nil {
			log.Printf("Failed to parse config for key %s: %v", configItem.Key, err)
			continue
		}

		// 创建服务器实例
		server := &ServerInstance{
			Key:           configItem.Key,
			Config:        v2Config,
			RawJSON:       rawJSON,
			Version:       configItem.Version,
			Status:        pb.ServerStatus_READY,
			IsTunOwner:    isOwner,
			LastHeartbeat: time.Now(),
		}

		servers[configItem.Key] = server
	}

	if len(servers) == 0 {
		return nil, fmt.Errorf("no valid server configurations found")
	}

	log.Printf("Successfully created %d server instances", len(servers))
	return servers, nil
}

// StartServer 启动单个服务器实例
func (x *RS) StartServer(server *ServerInstance) error {
	server.Status = pb.ServerStatus_STARTING
	server.LastError = ""

	// 创建上下文和取消函数
	_, cancelFunc := context.WithCancel(context.Background())
	server.CancelFunc = cancelFunc

	// 启动V2Ray实例
	instance, err := core.New(server.Config)
	if err != nil {
		server.Status = pb.ServerStatus_FAILED
		server.LastError = err.Error()
		return fmt.Errorf("failed to create V2Ray instance for key %s: %v", server.Key, err)
	}

	err = instance.Start()
	if err != nil {
		_ = instance.Close()
		server.Status = pb.ServerStatus_FAILED
		server.LastError = err.Error()
		return fmt.Errorf("failed to start V2Ray instance for key %s: %v", server.Key, err)
	}

	server.V2flyServer = instance
	server.Status = pb.ServerStatus_STARTED
	server.LastHeartbeat = time.Now()
	log.Printf("Successfully started server instance for key: %s", server.Key)

	return nil
}

// StopServer 停止单个服务器实例
func (x *RS) StopServer(server *ServerInstance) error {
	if server.V2flyServer != nil {
		err := server.V2flyServer.Close()
		if err != nil {
			log.Printf("Failed to close V2Ray instance for key %s: %v", server.Key, err)
			return err
		}
	}

	if server.CancelFunc != nil {
		server.CancelFunc()
	}

	server.Status = pb.ServerStatus_READY
	server.V2flyServer = nil
	server.LastError = ""
	log.Printf("Successfully stopped server instance for key: %s", server.Key)

	return nil
}

// snapshotServers 复制当前运行中的 server 引用，供热更新失败时回滚。
func (x *RS) snapshotServers() map[string]*ServerInstance {
	snap := make(map[string]*ServerInstance, len(x.Servers))
	for k, v := range x.Servers {
		snap[k] = v
	}
	return snap
}

// StartAllServers 启动所有服务器实例；仅在至少一个实例启动成功后才更新 Servers 与 TokenVersion。
func (x *RS) StartAllServers(servers map[string]*ServerInstance, tokenVersion int32) error {
	return x.startAllServers(servers, tokenVersion, true)
}

// StartAllServersNoReport 同 StartAllServers，但不向总台上报状态（供回滚路径使用）。
func (x *RS) StartAllServersNoReport(servers map[string]*ServerInstance, tokenVersion int32) error {
	return x.startAllServers(servers, tokenVersion, false)
}

func ensureTunBindInterface(profile rvstore.TunProfile) (string, error) {
	if iface := strings.TrimSpace(tunctl.CurrentBindInterface()); iface != "" {
		return iface, nil
	}
	iface, err := tunctl.SnapshotBindInterface(profile.BindInterface)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(iface) == "" {
		return "", fmt.Errorf("no physical interface detected")
	}
	return iface, nil
}

func collectServerRawJSONs(servers map[string]*ServerInstance) []string {
	raws := make([]string, 0, len(servers))
	for _, server := range servers {
		if server != nil && server.RawJSON != "" {
			raws = append(raws, server.RawJSON)
		}
	}
	return raws
}

func findTunOwnerKey(servers map[string]*ServerInstance) string {
	for key, server := range servers {
		if server != nil && server.IsTunOwner {
			return key
		}
	}
	return ""
}

func (x *RS) startAllServers(servers map[string]*ServerInstance, tokenVersion int32, report bool) error {
	profile := x.tunProfile()
	ownerKey := findTunOwnerKey(servers)
	allRaws := collectServerRawJSONs(servers)
	var ownerRaw string
	if owner := servers[ownerKey]; owner != nil {
		ownerRaw = owner.RawJSON
	}
	tunNeeded := profile.Use && ownerKey != "" && ownerRaw != ""
	log.Printf("TUN 启动检查: use=%v owner=%q needed=%v helper=%v",
		profile.Use, ownerKey, tunNeeded, tunctl.Default().HelperInstalled())

	if tunNeeded {
		if !tunctl.HasValidProxyListRoutes(ownerRaw) {
			return fmt.Errorf("proxy_list empty: refuse TUN (configure proxy_route_refs on server)")
		}
		if err := x.prepareTUNEnvironment(ownerKey, ownerRaw, allRaws, profile); err != nil {
			log.Printf("TUN 未就绪，降级为无 TUN 启动: %v", err)
			_ = tunctl.Default().Cleanup()
			if stripErr := stripTUNFromServers(servers); stripErr != nil {
				return fmt.Errorf("TUN degraded but strip failed: %w", stripErr)
			}
			x.setTunRuntime(ownerKey, false, err.Error())
			tunNeeded = false
		}
	}

	started := make(map[string]*ServerInstance, len(servers))
	ownerStarted := false
	var ownerDegraded string

	if tunNeeded {
		iface, _ := x.tunBindRuntimeLocked()
		if err := applyOutboundBindToServer(servers[ownerKey], iface); err != nil {
			log.Printf("TUN 拥有者出站 bindToDevice 注入失败: %v", err)
		}
		if err := x.StartServer(servers[ownerKey]); err != nil {
			ownerDegraded = err.Error()
			log.Printf("TUN 拥有者 %s 启动失败，降级为无 TUN: %v", ownerKey, err)
			_ = tunctl.Default().Cleanup()
			if stripErr := stripTUNFromServers(servers); stripErr != nil {
				log.Printf("strip TUN degrade: %v", stripErr)
			}
			x.setTunRuntime(ownerKey, false, ownerDegraded)
		} else {
			started[ownerKey] = servers[ownerKey]
			ownerStarted = true
			x.setTunRuntime(ownerKey, true, "")
		}
	}

	for key, server := range servers {
		if tunNeeded && key == ownerKey {
			continue
		}
		err := x.StartServer(server)
		if err != nil {
			log.Printf("Failed to start server %s: %v", key, err)
			continue
		}
		started[key] = server
	}

	if len(started) == 0 {
		_ = tunctl.Default().Cleanup()
		if !ownerStarted && tunNeeded && ownerDegraded != "" {
			x.setTunRuntime(ownerKey, false, ownerDegraded)
		}
		if report {
			x.reportAfterStart(servers, 0, len(servers))
		}
		return fmt.Errorf("failed to start any server instances")
	}

	if ownerStarted {
		if err := tunctl.Default().ApplyRoutes(); err != nil {
			log.Printf("apply TUN routes failed, rolling back: %v", err)
			if owner := started[ownerKey]; owner != nil {
				_ = x.StopServer(owner)
				delete(started, ownerKey)
			}
			_ = tunctl.Default().Cleanup()
			x.setTunRuntime(ownerKey, false, err.Error())
			ownerStarted = false
			if len(started) == 0 {
				if report {
					x.reportAfterStart(servers, 0, len(servers))
				}
				return fmt.Errorf("apply TUN routes: %w", err)
			}
			log.Printf("TUN 拥有者 %s 已回滚，其余 %d 个实例继续运行", ownerKey, len(started))
		} else if err := tunctl.Default().ApplyDNS(); err != nil {
			log.Printf("apply TUN DNS failed, rolling back: %v", err)
			if owner := started[ownerKey]; owner != nil {
				_ = x.StopServer(owner)
				delete(started, ownerKey)
			}
			_ = tunctl.Default().Cleanup()
			x.setTunRuntime(ownerKey, false, err.Error())
			ownerStarted = false
			if len(started) == 0 {
				if report {
					x.reportAfterStart(servers, 0, len(servers))
				}
				return fmt.Errorf("apply TUN DNS: %w", err)
			}
			log.Printf("TUN 拥有者 %s 已回滚，其余 %d 个实例继续运行", ownerKey, len(started))
		}
	}

	x.Servers = started
	x.TokenVersion = tokenVersion
	log.Printf("Successfully started %d/%d server instances", len(started), len(servers))
	if tunNeeded && !ownerStarted {
		log.Printf("TUN 已降级：拥有者 %s 未启动，其余实例无全局 TUN", ownerKey)
	}
	x.markStarted()
	if report {
		x.reportAfterStart(servers, len(started), len(servers))
	}
	return nil
}

// StopAllServers 停止所有服务器实例
func (x *RS) StopAllServers() {
	for key, server := range x.Servers {
		err := x.StopServer(server)
		if err != nil {
			log.Printf("Error stopping server %s: %v", key, err)
		}
	}
	x.Servers = make(map[string]*ServerInstance)
	_ = tunctl.Default().Cleanup()
	tunctl.ResetBindInterface()
	tunctl.DisableDialBind()
	x.setTunRuntime("", false, "")
	x.setTunBindRuntime("", false)
	log.Printf("All server instances stopped")
}
func (x *RS) UploadExecuteError(ctx context.Context, c *pb.Execute, uploadErr error) {
	x.reportExecuteResult(ctx, c, uploadErr)
}

// Execute 执行命令
func (x *RS) Execute(ctx context.Context, c []*pb.Execute) {
	for i, execute := range c {
		log.Printf("执行%v", i)
		if execute.OrderType == pb.OrderType_WRITEFILE {
			create, err := os.Create(execute.FilePath)
			if err != nil {
				go x.UploadExecuteError(ctx, execute, err)
				log.Printf("打开文件失败%v", err)
				break
			}
			defer create.Close()
			_, err = create.WriteString(execute.Content)
			if err != nil {
				go x.UploadExecuteError(ctx, execute, err)
				log.Printf("写入文件失败%v", err)
				break
			}
			go x.UploadExecuteError(ctx, execute, err)
		} else if execute.OrderType == pb.OrderType_EXECUTE {
			_, err := exec.Command(execute.BinInfo, execute.Args...).Output()

			if err != nil {
				go x.UploadExecuteError(ctx, execute, err)
				log.Printf("执行命令失败%v", err)
				break
			}
			go x.UploadExecuteError(ctx, execute, err)
		}
	}
}
