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
	wireDialing     bool
	ifaceGone       bool
	cnOutage        bool
	cnOutageGateway string
	physicalGateway string
}

// cnOutageActive 国内断网只对探测当时的出口有效（网关 + 本机地址）。换了其中任意一个，上次结论不再算。
func cnOutageActive(outage bool, probed, current string) bool {
	return outage && probed != "" && probed == current
}

func liveHijackInputs() hijackInputs {
	hijackMu.Lock()
	bits := hijackBits
	hijackMu.Unlock()
	return hijackInputs{
		enabled:     Default().Status().Enabled,
		wireDialing: bits.wireDialing,
		ifaceGone:   bits.ifaceGone,
		cnOutage:    cnOutageActive(bits.cnOutage, bits.cnOutageGateway, bits.physicalGateway),
	}
}

func syncHijack() error {
	in := liveHijackInputs()
	var err error
	if hijackWanted(in) {
		err = defaultManager.ResumeRoutes()
	} else {
		err = defaultManager.SuspendRoutes()
	}
	reportHijackAnomaly(in)
	return err
}

// WithWireDial hello 期间不劫持，避免控制面进 TUN。
// 成功：当前网关可用。失败：没有默认网关则不探测；有网关才记国内断网，且只对这个网关有效。
func WithWireDial(fn func() error) error {
	hijackMu.Lock()
	hijackBits.wireDialing = true
	hijackMu.Unlock()
	_ = syncHijack()

	err := fn()

	hijackMu.Lock()
	hijackBits.wireDialing = false
	gw := hijackBits.physicalGateway
	gone := hijackBits.ifaceGone
	if err == nil {
		hijackBits.cnOutage = false
		hijackBits.cnOutageGateway = gw
	}
	hijackMu.Unlock()
	if err != nil && !gone && gw != "" && Default().Status().Enabled {
		outage := DomesticMassOutage()
		hijackMu.Lock()
		hijackBits.cnOutage = outage
		hijackBits.cnOutageGateway = gw
		hijackMu.Unlock()
	}
	_ = syncHijack()
	return err
}

func noteDefaultIface(name string) {
	hijackMu.Lock()
	hijackBits.ifaceGone = name == ""
	hijackMu.Unlock()
	_ = syncHijack()
}

func noteGatewayAbsent() {
	hijackMu.Lock()
	hijackBits.ifaceGone = true
	hijackMu.Unlock()
}

func noteGateway(gw string) {
	hijackMu.Lock()
	hijackBits.ifaceGone = false
	hijackBits.physicalGateway = gw
	hijackMu.Unlock()
}

func resetHijackBits() {
	hijackMu.Lock()
	hijackBits.wireDialing = false
	hijackBits.ifaceGone = false
	hijackBits.cnOutage = false
	hijackBits.cnOutageGateway = ""
	hijackMu.Unlock()
}
