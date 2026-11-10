package tunctl

import (
	"log"
	"sync"
	"time"
)

// 劫持路由的唯一真相：用户开着 TUN 且引擎在跑、有默认网卡、国内不是大面积断网。
// 控制面重连不暂停劫持。用户开关是 TunProfile.Use（Enabled）；本状态机只决定「要不要装默认路由」，不会把关掉的 TUN 打开。

type hijackInputs struct {
	enabled   bool
	ifaceGone bool
	cnOutage  bool
}

func hijackWanted(in hijackInputs) bool {
	return in.enabled && !in.ifaceGone && !in.cnOutage
}

const (
	HijackPausedUserOff   = "user_off"
	HijackPausedEngineOff = "engine_off"
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
	ifaceGone       bool
	cnOutage        bool
	cnOutageGateway string
	physicalGateway string
}

var (
	ifaceGoneDebounce = 5 * time.Second
	ifaceGoneTimer    *time.Timer
	ifaceGoneSince    time.Time
	ifaceGoneGen      uint64

	cnOutageRecheckEvery = 20 * time.Second
	cnOutageRecheck      *time.Timer
	cnOutageRecheckGen   uint64

	hijackLogMu       sync.Mutex
	hijackPausedSince time.Time
	hijackLastReason  string
)

// cnOutageActive 国内断网只对探测当时的出口有效（网关 + 本机地址）。换了其中任意一个，上次结论不再算。
func cnOutageActive(outage bool, probed, current string) bool {
	return outage && probed != "" && probed == current
}

func liveHijackInputs() hijackInputs {
	hijackMu.Lock()
	bits := hijackBits
	hijackMu.Unlock()
	return hijackInputs{
		enabled:   Default().Status().Enabled,
		ifaceGone: bits.ifaceGone,
		cnOutage:  cnOutageActive(bits.cnOutage, bits.cnOutageGateway, bits.physicalGateway),
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
	logHijackTransition(in)
	return err
}

func logHijackTransition(in hijackInputs) {
	reason := hijackPausedReason(in)
	hijackLogMu.Lock()
	defer hijackLogMu.Unlock()
	if reason == "" {
		if hijackPausedSince.IsZero() {
			return
		}
		dur := time.Since(hijackPausedSince).Round(time.Millisecond)
		hijackPausedSince = time.Time{}
		hijackLastReason = ""
		log.Printf("劫持恢复: 暂停了 %s", dur)
		return
	}
	if reason == hijackLastReason {
		return
	}
	if hijackPausedSince.IsZero() {
		hijackPausedSince = time.Now()
	}
	hijackLastReason = reason
	log.Printf("劫持暂停: %s", HijackPausedZH(reason))
}

// NoteWireDialResult 记录控制面拨号结果，不改路由。
// 有默认网关且拨号失败时才探测国内连通性。拨号成功，或周期重探恢复，都会清掉国内断网。
func NoteWireDialResult(err error) {
	if err == nil {
		hijackMu.Lock()
		changed := hijackBits.cnOutage
		hijackBits.cnOutage = false
		hijackBits.cnOutageGateway = hijackBits.physicalGateway
		stopOutageRecheckLocked()
		hijackMu.Unlock()
		if changed {
			_ = syncHijack()
		}
		return
	}

	hijackMu.Lock()
	gw := hijackBits.physicalGateway
	gone := hijackBits.ifaceGone
	hijackMu.Unlock()
	if gone || gw == "" || !Default().Status().Enabled {
		return
	}

	outage := DomesticMassOutage()
	hijackMu.Lock()
	if hijackBits.ifaceGone || hijackBits.physicalGateway != gw {
		hijackMu.Unlock()
		return
	}
	prev := hijackBits.cnOutage
	hijackBits.cnOutage = outage
	hijackBits.cnOutageGateway = gw
	if outage {
		startOutageRecheckLocked(gw)
	} else {
		stopOutageRecheckLocked()
	}
	hijackMu.Unlock()
	if prev != outage {
		_ = syncHijack()
	}
}

func startOutageRecheckLocked(gw string) {
	if cnOutageRecheck != nil {
		return
	}
	cnOutageRecheckGen++
	gen := cnOutageRecheckGen
	cnOutageRecheck = time.AfterFunc(cnOutageRecheckEvery, func() { recheckCNOutage(gen, gw) })
}

func stopOutageRecheckLocked() {
	cnOutageRecheckGen++
	if cnOutageRecheck != nil {
		cnOutageRecheck.Stop()
		cnOutageRecheck = nil
	}
}

func recheckCNOutage(gen uint64, gw string) {
	hijackMu.Lock()
	if gen != cnOutageRecheckGen || !hijackBits.cnOutage || hijackBits.physicalGateway != gw {
		cnOutageRecheck = nil
		hijackMu.Unlock()
		return
	}
	hijackMu.Unlock()

	outage := DomesticMassOutage()

	hijackMu.Lock()
	if gen != cnOutageRecheckGen || hijackBits.physicalGateway != gw {
		cnOutageRecheck = nil
		hijackMu.Unlock()
		return
	}
	cnOutageRecheck = nil
	if outage {
		hijackBits.cnOutage = true
		hijackBits.cnOutageGateway = gw
		startOutageRecheckLocked(gw)
		hijackMu.Unlock()
		return
	}
	hijackBits.cnOutage = false
	hijackMu.Unlock()
	log.Printf("国内连通性恢复")
	_ = syncHijack()
}

func noteDefaultIface(name string) {
	hijackMu.Lock()
	if name == "" {
		if ifaceGoneTimer != nil || hijackBits.ifaceGone {
			hijackMu.Unlock()
			return
		}
		ifaceGoneSince = time.Now()
		ifaceGoneGen++
		gen := ifaceGoneGen
		ifaceGoneTimer = time.AfterFunc(ifaceGoneDebounce, func() { confirmIfaceGone(gen) })
		hijackMu.Unlock()
		log.Printf("默认网卡消失")
		return
	}

	timer := ifaceGoneTimer
	ifaceGoneTimer = nil
	since := ifaceGoneSince
	wasGone := hijackBits.ifaceGone
	if timer != nil {
		ifaceGoneGen++
	}
	if wasGone {
		hijackBits.ifaceGone = false
	}
	ifaceGoneSince = time.Time{}
	hijackMu.Unlock()
	if timer != nil {
		timer.Stop()
	}
	if timer == nil && !wasGone {
		return
	}
	if wasGone {
		if since.IsZero() {
			log.Printf("默认网卡恢复 %s", name)
		} else {
			log.Printf("默认网卡恢复 %s 断开 %s", name, time.Since(since).Round(time.Millisecond))
		}
		_ = syncHijack()
		return
	}
	log.Printf("默认网卡恢复 %s 断开 %s（未达去抖阈值，未暂停）", name, time.Since(since).Round(time.Millisecond))
}

func confirmIfaceGone(gen uint64) {
	hijackMu.Lock()
	if gen != ifaceGoneGen {
		hijackMu.Unlock()
		return
	}
	ifaceGoneTimer = nil
	hijackBits.ifaceGone = true
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
	hijackBits.ifaceGone = false
	hijackBits.cnOutage = false
	hijackBits.cnOutageGateway = ""
	timer := ifaceGoneTimer
	ifaceGoneTimer = nil
	ifaceGoneGen++
	ifaceGoneSince = time.Time{}
	stopOutageRecheckLocked()
	hijackMu.Unlock()
	if timer != nil {
		timer.Stop()
	}
	hijackLogMu.Lock()
	hijackPausedSince = time.Time{}
	hijackLastReason = ""
	hijackLogMu.Unlock()
}
