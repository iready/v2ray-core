package localadmin

import (
	"runtime"
	"sort"
	"time"

	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
	"github.com/v2fly/v2ray-core/v5/main/z/regService"
	"github.com/v2fly/v2ray-core/v5/main/z/rocket"
	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
)

// ServerInstanceStatus 单个 v2ray 实例健康快照。
type ServerInstanceStatus struct {
	Key           string `json:"key"`
	Status        string `json:"status"`
	Version       int32  `json:"version"`
	Healthy       bool   `json:"healthy"`
	LastHeartbeat string `json:"last_heartbeat,omitempty"`
	LastError     string `json:"last_error,omitempty"`
}

// StatusSnapshot 本地后台状态 API 响应。
type StatusSnapshot struct {
	WireConnected bool                   `json:"wire_connected"`
	TokenVersion  int32                  `json:"token_version"`
	ServerKeys    []string               `json:"server_keys"`
	ServerCount   int                    `json:"server_count"`
	HealthyCount  int                    `json:"healthy_count"`
	Servers       []ServerInstanceStatus `json:"servers"`
	LastError     string                 `json:"last_error"`
	ConnectedAt   string                 `json:"connected_at,omitempty"`
	StartedAt     string                 `json:"started_at,omitempty"`
	ConfigPath    string                 `json:"config_path"`
	AgentReady    bool                   `json:"agent_ready"`
	AgentPID      int                    `json:"agent_pid"`
	AdminPort     int                    `json:"admin_port"`
	Addr          string                 `json:"addr,omitempty"`
	TunEnabled    bool                   `json:"tun_enabled"`
	TunHelper     bool                   `json:"tun_helper_installed"`
	TunIfName     string                 `json:"tun_if_name,omitempty"`
	TunInConfig   bool                   `json:"tun_in_config"`
	TunLocalUse   bool                   `json:"tun_local_use"`
	TunOwnerKey   string                 `json:"tun_owner_key,omitempty"`
	TunActive     bool                   `json:"tun_active"`
	TunDegraded   string                 `json:"tun_degraded_reason,omitempty"`
	TunBindIface  string                 `json:"tun_bind_interface,omitempty"`
	TunBindAuto   bool                   `json:"tun_bind_auto"`
	Elevated      bool                   `json:"elevated"`
	Autostart     bool                   `json:"autostart_installed"`
	ClientBuild   string                 `json:"client_build,omitempty"`
	ExePath       string                 `json:"exe_path,omitempty"`
	OS            string                 `json:"os"`
	Features      PlatformFeatures       `json:"features"`
}

// PlatformFeatures 管理后台按本机系统展示的能力开关。
type PlatformFeatures struct {
	PrivilegedHelper bool `json:"privileged_helper"` // macOS Privileged Helper
	ElevatedAuth     bool `json:"elevated_auth"`     // Windows 管理员 / UAC 提权自启
	LoginAutostart   bool `json:"login_autostart"`   // 登录自启（Win 计划任务 / Mac LaunchAgent）
}

func serverHealthy(srv *rocket.ServerInstance) bool {
	return srv != nil &&
		srv.Status == pb.ServerStatus_STARTED &&
		srv.V2flyServer != nil
}

func buildServerStatuses(rs *rocket.RS) []ServerInstanceStatus {
	if rs == nil || len(rs.Servers) == 0 {
		return nil
	}
	out := make([]ServerInstanceStatus, 0, len(rs.Servers))
	for key, srv := range rs.Servers {
		if srv == nil {
			continue
		}
		inst := ServerInstanceStatus{
			Key:       key,
			Status:    srv.Status.String(),
			Version:   srv.Version,
			Healthy:   serverHealthy(srv),
			LastError: srv.LastError,
		}
		if !srv.LastHeartbeat.IsZero() {
			inst.LastHeartbeat = srv.LastHeartbeat.UTC().Format(time.RFC3339)
		}
		out = append(out, inst)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func platformFeatures() PlatformFeatures {
	switch runtime.GOOS {
	case "windows":
		return PlatformFeatures{ElevatedAuth: true, LoginAutostart: true}
	case "darwin":
		return PlatformFeatures{PrivilegedHelper: true, LoginAutostart: true}
	default:
		return PlatformFeatures{}
	}
}

func BuildStatus(rs *rocket.RS, cfg AgentConfig, rt rvstore.Runtime) StatusSnapshot {
	snap := StatusSnapshot{
		ConfigPath:  AgentConfigPath(),
		AgentReady:  cfg.IsComplete(),
		Addr:        cfg.ResolvedAddr(),
		Servers:     []ServerInstanceStatus{},
		AgentPID:    rt.PID,
		AdminPort:   rt.AdminPort,
		Elevated:    tunctl.IsElevated(),
		Autostart:   regService.AutoStartInstalled(),
		ClientBuild: ClientBuild,
		ExePath:     selfExePath(),
		OS:          runtime.GOOS,
		Features:    platformFeatures(),
	}
	if rs == nil {
		return snap
	}
	st := rs.Status()
	snap.WireConnected = st.WireConnected
	snap.TokenVersion = rs.TokenVersion
	snap.LastError = st.LastWireError
	if !st.ConnectedSince.IsZero() {
		snap.ConnectedAt = st.ConnectedSince.UTC().Format(time.RFC3339)
	}
	if !st.StartedAt.IsZero() {
		snap.StartedAt = st.StartedAt.UTC().Format(time.RFC3339)
	}
	snap.Servers = buildServerStatuses(rs)
	snap.ServerCount = len(snap.Servers)
	for _, inst := range snap.Servers {
		if inst.Healthy {
			snap.HealthyCount++
		}
		snap.ServerKeys = append(snap.ServerKeys, inst.Key)
	}
	tunSt := tunctl.Default().Status()
	snap.TunEnabled = tunSt.Enabled
	snap.TunHelper = tunSt.HelperInstalled
	snap.TunIfName = tunSt.IfName
	snap.TunInConfig = rocket.ConfigHasTUN(rs, nil)
	snap.TunOwnerKey = st.TunOwnerKey
	snap.TunActive = st.TunActive
	snap.TunDegraded = st.TunDegradedReason
	snap.TunBindIface = tunctl.CurrentBindInterface()
	if snap.TunBindIface == "" {
		snap.TunBindIface = st.TunBindInterface
	}
	snap.TunBindAuto = tunctl.BindInterfaceAuto() || st.TunBindAuto
	return snap
}
