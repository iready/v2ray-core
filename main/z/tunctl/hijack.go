package tunctl

import "sync"

// 劫持路由的唯一真相：用户开着 TUN 且引擎在跑、没在 hello、有默认网卡、国内不是大面积断网。
// 用户开关是 TunProfile.Use（Enabled）；本状态机只决定「要不要装默认路由」，不会把关掉的 TUN 打开。

type hijackInputs struct {
	enabled     bool
	wireDialing bool
	ifaceGone   bool
	cnOutage    bool
}

func hijackWanted(in hijackInputs) bool {
	return in.enabled && !in.wireDialing && !in.ifaceGone && !in.cnOutage
}

const (
	HijackPausedUserOff   = "user_off"
	HijackPausedEngineOff = "engine_off"
	HijackPausedWireDial  = "wire_dial"
	HijackPausedIfaceGone = "iface_gone"
	HijackPausedCNOutage  = "cn_outage"
)

// HijackSnapshot 给本地后台：是否正在劫持默认路由、若否则原因。
type HijackSnapshot struct {
	Wanted bool   `json:"wanted"`
	Paused string `json:"paused,omitempty"`
}

func HijackSnapshotNow() HijackSnapshot {
	in := liveHijackInputs()
	if hijackWanted(in) {
		return HijackSnapshot{Wanted: true}
	}
	return HijackSnapshot{Paused: hijackPausedReason(in)}
}

func hijackPausedReason(in hijackInputs) string {
	if hijackWanted(in) {
		return ""
	}
	if !in.enabled {
		if userOptedOut() {
			return HijackPausedUserOff
		}
		return HijackPausedEngineOff
	}
	if in.wireDialing {
		return HijackPausedWireDial
	}
	if in.ifaceGone {
		return HijackPausedIfaceGone
	}
	if in.cnOutage {
		return HijackPausedCNOutage
	}
	return HijackPausedEngineOff
}

// HijackPausedZH 后台/诊断用的暂停原因文案；空串表示正在劫持。
func HijackPausedZH(paused string) string {
	switch paused {
	case HijackPausedUserOff:
		return "用户已关闭"
	case HijackPausedEngineOff:
		return "网卡未运行"
	case HijackPausedWireDial:
		return "控制面重连，暂直出"
	case HijackPausedIfaceGone:
		return "默认网卡丢失，暂直出"
	case HijackPausedCNOutage:
		return "国内大面积不通，暂直出"
	case "":
		return ""
	default:
		return paused
	}
}

var hijackMu sync.Mutex
var hijackBits struct {
	wireDialing bool
	ifaceGone   bool
	cnOutage    bool
}

func liveHijackInputs() hijackInputs {
	hijackMu.Lock()
	bits := hijackBits
	hijackMu.Unlock()
	return hijackInputs{
		enabled:     Default().Status().Enabled,
		wireDialing: bits.wireDialing,
		ifaceGone:   bits.ifaceGone,
		cnOutage:    bits.cnOutage,
	}
}

func syncHijack() error {
	if hijackWanted(liveHijackInputs()) {
		return defaultManager.ResumeRoutes()
	}
	return defaultManager.SuspendRoutes()
}

// WithWireDial hello 期间不劫持，避免控制面进 TUN。
// 成功：国内未断网，恢复劫持。失败：先探测国内站，通则恢复（服务端更新），不通才 fail-open。
func WithWireDial(fn func() error) error {
	hijackMu.Lock()
	hijackBits.wireDialing = true
	hijackMu.Unlock()
	_ = syncHijack()

	err := fn()

	hijackMu.Lock()
	hijackBits.wireDialing = false
	if err == nil {
		hijackBits.cnOutage = false
	}
	hijackMu.Unlock()
	if err != nil && Default().Status().Enabled {
		outage := DomesticMassOutage()
		hijackMu.Lock()
		hijackBits.cnOutage = outage
		hijackMu.Unlock()
	}
	_ = syncHijack()
	return err
}

func noteDefaultIface(name string) {
	gone := name == ""
	hijackMu.Lock()
	hijackBits.ifaceGone = gone
	hijackMu.Unlock()
	if gone {
		_ = syncHijack()
		return
	}
	if Default().Status().Enabled {
		outage := DomesticMassOutage()
		hijackMu.Lock()
		hijackBits.cnOutage = outage
		hijackMu.Unlock()
	}
	_ = syncHijack()
}

func resetHijackBits() {
	hijackMu.Lock()
	hijackBits.wireDialing = false
	hijackBits.ifaceGone = false
	hijackBits.cnOutage = false
	hijackMu.Unlock()
}
