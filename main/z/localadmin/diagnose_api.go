package localadmin

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
	"github.com/v2fly/v2ray-core/v5/main/z/rocket"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
	"github.com/v2fly/v2ray-core/v5/main/z/wire"
)

func selfExePath() string {
	p, err := os.Executable()
	if err != nil {
		return "?"
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// DiagnoseCheck 单项排查结果。
type DiagnoseCheck struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	OK      bool   `json:"ok"`
	Level   string `json:"level"` // ok | warn | fail
	Detail  string `json:"detail"`
	Hint    string `json:"hint,omitempty"`
	Elapsed string `json:"elapsed,omitempty"`
}

// DiagnoseReport 一键排查报告。
type DiagnoseReport struct {
	OK      bool            `json:"ok"`
	Summary string          `json:"summary"`
	Verdict string          `json:"verdict,omitempty"` // 根因一句话
	Cause   string          `json:"cause,omitempty"`   // 机器可读原因码
	Fix     string          `json:"fix,omitempty"`     // 建议操作
	Checks  []DiagnoseCheck `json:"checks"`
}

func (h *Handler) PostDiagnose(c *gin.Context) {
	report := h.runDiagnose()
	c.JSON(http.StatusOK, report)
}

func (h *Handler) runDiagnose() DiagnoseReport {
	checks := make([]DiagnoseCheck, 0, 12)
	cfg := h.hooks.GetConfig()
	rs := h.hooks.GetRS()
	snap := BuildStatus(rs, cfg, h.store.LoadRuntime())
	if cached, err := h.store.RVStore().LoadConfigCache(); err == nil && cached != nil && cached.Config != nil {
		snap.TunInConfig = rocket.ConfigHasTUN(rs, cached.Config)
	}
	if profile, err := h.store.LoadTunProfile(); err == nil {
		snap.TunLocalUse = profile.Use
	}

	checks = append(checks, DiagnoseCheck{
		ID:     "build",
		Title:  "客户端标记",
		OK:     true,
		Level:  "ok",
		Detail: ClientBuild + " · " + selfExePath(),
	})
	checks = append(checks, h.checkRuntimeSnapshot(snap))
	checks = append(checks, checkElevated())
	checks = append(checks, checkWire(snap))
	checks = append(checks, checkInstances(snap))
	checks = append(checks, checkTun(snap))
	checks = append(checks, h.checkBindInterface(snap))
	checks = append(checks, checkStaleDNS(snap))
	checks = append(checks, checkCNDNS(snap))
	tunOn := snap.TunActive || snap.TunEnabled
	if tunOn {
		// TUN 下仅 TCP 建连会被 FakeDNS 假 IP「秒过」，必须拉真实 HTTPS。
		checks = append(checks, checkHTTPSGet("https_cn", "国内 HTTPS (www.baidu.com)",
			"https://www.baidu.com/", cnHTTPSHint(snap)))
		checks = append(checks, checkHTTPSGet("https_proxy", "外网 HTTPS (www.google.com)",
			"https://www.google.com/generate_204", proxyHTTPSHint(snap)))
		checks = append(checks, checkForeignIPProxy(snap))
	} else {
		checks = append(checks, checkTCP("https_cn", "国内 HTTPS (www.baidu.com:443)", "www.baidu.com:443",
			cnHTTPSHint(snap)))
		checks = append(checks, checkTCP("https_proxy", "外网 HTTPS (www.google.com:443)", "www.google.com:443",
			proxyHTTPSHint(snap)))
	}

	if addr := strings.TrimSpace(snap.Addr); addr != "" {
		host := wire.HostFromURL(addr)
		if host != "" {
			port := wire.PortFromURL(addr)
			if port == "" {
				port = "443"
			}
			// Linux Docker：TUN 改 resolv 后容器名可能短时不可解析；Wire 已通则控制面实测已通。
			if runtime.GOOS == "linux" && snap.WireConnected {
				checks = append(checks, DiagnoseCheck{
					ID:     "wire_host",
					Title:  "Rocket 控制面 " + host + ":" + port,
					OK:     true,
					Level:  "ok",
					Detail: "Wire 已连接（跳过主机名重解析）",
				})
			} else {
				checks = append(checks, checkTCP("wire_host", "Rocket 控制面 "+host+":"+port, net.JoinHostPort(host, port),
					"控制面不通：无法拉配置/重连，检查防火墙或服务端"))
			}
		}
	}

	// TUN 关闭时外网直连失败属预期，降为 warn，避免误报 FAILED。
	for i := range checks {
		if checks[i].ID == "https_proxy" && !checks[i].OK && !tunOn {
			checks[i].Level = "warn"
			checks[i].Hint = "TUN 已关，外网直连失败正常；开启 TUN 后再测代理/FakeDNS"
		}
	}

	fail, warn := 0, 0
	byID := make(map[string]DiagnoseCheck, len(checks))
	for _, ch := range checks {
		byID[ch.ID] = ch
		switch ch.Level {
		case "fail":
			fail++
		case "warn":
			warn++
		}
	}
	verdict, cause, fix := synthesizeVerdict(tunOn, byID)
	summary := fmt.Sprintf("共 %d 项：通过 %d，警告 %d，失败 %d", len(checks), len(checks)-fail-warn, warn, fail)
	if verdict != "" {
		summary = summary + "｜根因：" + verdict
	}
	return DiagnoseReport{
		OK:      fail == 0,
		Summary: summary,
		Verdict: verdict,
		Cause:   cause,
		Fix:     fix,
		Checks:  checks,
	}
}

func checkElevated() DiagnoseCheck {
	start := time.Now()
	if runtime.GOOS != "windows" {
		return DiagnoseCheck{
			ID:      "elevated",
			Title:   "管理员权限",
			OK:      true,
			Level:   "ok",
			Detail:  "非 Windows，无需 UAC 提权检查",
			Elapsed: time.Since(start).Round(time.Millisecond).String(),
		}
	}
	ok := tunctl.IsElevated()
	ch := DiagnoseCheck{
		ID:      "elevated",
		Title:   "管理员权限",
		OK:      ok,
		Elapsed: time.Since(start).Round(time.Millisecond).String(),
	}
	if ok {
		ch.Level = "ok"
		ch.Detail = "已提权，可改路由/DNS/创建 TUN"
	} else {
		ch.Level = "fail"
		ch.Detail = "未以管理员运行"
		ch.Hint = "打开后台「运行状态 → 权限与自启」：开启提权自启，再点「立即提权重启」"
	}
	return ch
}

func checkWire(snap StatusSnapshot) DiagnoseCheck {
	start := time.Now()
	ch := DiagnoseCheck{ID: "wire", Title: "Rocket 控制面连接", Elapsed: time.Since(start).Round(time.Millisecond).String()}
	if !snap.AgentReady {
		ch.OK = false
		ch.Level = "warn"
		ch.Detail = "Agent 未就绪"
		ch.Hint = "先在「设置」填好地址与 token"
		return ch
	}
	if snap.WireConnected {
		ch.OK = true
		ch.Level = "ok"
		ch.Detail = "已连接"
		if snap.Addr != "" {
			ch.Detail += " " + snap.Addr
		}
		return ch
	}
	ch.OK = false
	ch.Level = "fail"
	ch.Detail = "未连接"
	if snap.LastError != "" {
		ch.Detail += "：" + snap.LastError
	}
	ch.Hint = "点「立即重连」；若长期失败检查网络与 token"
	return ch
}

func checkInstances(snap StatusSnapshot) DiagnoseCheck {
	ch := DiagnoseCheck{
		ID:     "instances",
		Title:  "v2ray 实例",
		Detail: fmt.Sprintf("健康 %d / 总数 %d", snap.HealthyCount, snap.ServerCount),
	}
	if snap.ServerCount == 0 {
		ch.OK = false
		ch.Level = "fail"
		ch.Hint = "配置未下发或启动失败，看状态页最近错误"
		return ch
	}
	if snap.HealthyCount == 0 {
		ch.OK = false
		ch.Level = "fail"
		ch.Hint = "实例全挂：端口占用、配置错误或 TUN 降级后未恢复"
		return ch
	}
	if snap.HealthyCount < snap.ServerCount {
		ch.OK = true
		ch.Level = "warn"
		ch.Hint = "部分实例失败；若含 TUN 拥有者，外网可能异常"
		return ch
	}
	ch.OK = true
	ch.Level = "ok"
	return ch
}

func checkTun(snap StatusSnapshot) DiagnoseCheck {
	ch := DiagnoseCheck{ID: "tun", Title: "TUN 状态"}
	if !snap.TunLocalUse {
		ch.OK = true
		ch.Level = "ok"
		ch.Detail = "本地已关闭 TUN（仅代理端口模式）"
		return ch
	}
	if snap.TunActive && snap.TunEnabled {
		ch.OK = true
		ch.Level = "ok"
		ch.Detail = "TUN 已生效"
		if snap.TunIfName != "" {
			ch.Detail += "（" + snap.TunIfName + "）"
		}
		return ch
	}
	ch.OK = false
	ch.Level = "fail"
	if snap.TunDegraded != "" {
		ch.Detail = snap.TunDegraded
	} else {
		ch.Detail = "本地已开 TUN 但未生效"
	}
	ch.Hint = "常见：tun0 残留、端口占用；可关开 TUN 或重启"
	if runtime.GOOS == "darwin" {
		ch.Hint = "常见：未装 Helper、utun 残留；后台 TUN 页安装 Helper 后重试"
	} else if runtime.GOOS == "windows" {
		ch.Hint = "常见：未提权、tun0 残留、端口占用；可关开 TUN 或以管理员运行"
	}
	return ch
}

func (h *Handler) checkBindInterface(snap StatusSnapshot) DiagnoseCheck {
	start := time.Now()
	ch := DiagnoseCheck{ID: "bind_iface", Title: "出站绑网卡", Elapsed: time.Since(start).Round(time.Millisecond).String()}
	profile, _ := h.store.LoadTunProfile()
	candidates := tunctl.ListBindInterfaceCandidates()
	effective := snap.TunBindIface
	if effective == "" {
		effective = tunctl.CurrentBindInterface()
	}
	if effective == "" {
		effective = strings.TrimSpace(profile.BindInterface)
	}
	ch.Elapsed = time.Since(start).Round(time.Millisecond).String()
	ch.Detail = fmt.Sprintf("当前=%q 候选=%v", effective, candidates)
	if len(candidates) == 0 {
		ch.OK = false
		ch.Level = "fail"
		ch.Hint = "没有可用物理网卡，TUN 准备会重试卡住"
		return ch
	}
	if effective == "" {
		ch.OK = true
		ch.Level = "ok"
		ch.Detail = "自动探测；候选 " + strings.Join(candidates, ", ")
		return ch
	}
	if tunctl.BindInterfaceKnown(effective) {
		ch.OK = true
		ch.Level = "ok"
		return ch
	}
	ch.OK = false
	ch.Level = "fail"
	ch.Detail = fmt.Sprintf("配置网卡 %q 不在本机候选中：%v", effective, candidates)
	ch.Hint = "到「TUN」页清空绑网卡或改选本机真实网卡名（别机拷来的「以太网」常不对）"
	return ch
}

func checkStaleDNS(snap StatusSnapshot) DiagnoseCheck {
	start := time.Now()
	stale := tunctl.StaleLoopbackDNSInterfaces()
	ch := DiagnoseCheck{
		ID:      "stale_dns",
		Title:   "系统 DNS 残留",
		Elapsed: time.Since(start).Round(time.Millisecond).String(),
	}
	// TUN 生效时适配器 DNS 由 sing-tun 管理；物理网卡不应再被钉 127.0.0.1。
	if snap.TunActive || snap.TunEnabled {
		if len(stale) == 0 {
			ch.OK = true
			ch.Level = "ok"
			ch.Detail = "TUN 运行中；物理网卡无异常 DNS 钉死"
			return ch
		}
		ch.OK = false
		ch.Level = "warn"
		ch.Detail = "TUN 已开，但物理网卡仍钉 127.0.0.1（旧版本残留）：" + strings.Join(stale, ", ")
		ch.Hint = "关 TUN 后重启 rocket 会自动恢复 DHCP；或手动把网卡 DNS 改回自动获取"
		return ch
	}
	if len(stale) == 0 {
		ch.OK = true
		ch.Level = "ok"
		ch.Detail = "未发现 DNS 仅指向 127.0.0.1 的网卡"
		return ch
	}
	ch.OK = false
	ch.Level = "fail"
	ch.Detail = "以下网卡 DNS 残留本机回环：" + strings.Join(stale, ", ")
	ch.Hint = "以管理员重启 rocket，或手动把网卡 DNS 改回自动获取"
	return ch
}

func checkTCP(id, title, address, hint string) DiagnoseCheck {
	start := time.Now()
	ch := DiagnoseCheck{ID: id, Title: title, Hint: hint}
	d := net.Dialer{Timeout: 4 * time.Second}
	conn, err := d.DialContext(context.Background(), "tcp", address)
	ch.Elapsed = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		ch.OK = false
		ch.Level = "fail"
		ch.Detail = err.Error()
		return ch
	}
	_ = conn.Close()
	ch.OK = true
	ch.Level = "ok"
	ch.Detail = "TCP 连通"
	return ch
}

func diagnoseHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           (&net.Dialer{Timeout: 4 * time.Second}).DialContext,
			TLSHandshakeTimeout:   4 * time.Second,
			ResponseHeaderTimeout: 4 * time.Second,
			ForceAttemptHTTP2:     false,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func resolveHostIPs(host string) (ips []string, fake bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, false
	}
	for _, a := range addrs {
		ips = append(ips, a)
		if addr, err := netip.ParseAddr(a); err == nil && singtun.IsFakeDNSAddr(addr) {
			fake = true
		}
	}
	return ips, fake
}

// checkHTTPSGet 走系统默认路由发真实 HTTPS（TUN 开着时即测 FakeDNS+代理整条链）。
func checkHTTPSGet(id, title, rawURL, hint string) DiagnoseCheck {
	start := time.Now()
	ch := DiagnoseCheck{ID: id, Title: title, Hint: hint}
	host := ""
	if u, err := http.NewRequest(http.MethodGet, rawURL, nil); err == nil && u.URL != nil {
		host = u.URL.Hostname()
	}
	var resolveNote string
	if host != "" {
		ips, fake := resolveHostIPs(host)
		if len(ips) > 0 {
			resolveNote = fmt.Sprintf("解析 %v", ips)
			if fake {
				resolveNote += "（含 FakeDNS）"
			}
		}
	}
	resp, err := diagnoseHTTPClient().Get(rawURL)
	ch.Elapsed = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		ch.OK = false
		ch.Level = "fail"
		ch.Detail = err.Error()
		if resolveNote != "" {
			ch.Detail = resolveNote + "；" + ch.Detail
		}
		return ch
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
	ch.OK = true
	ch.Level = "ok"
	ch.Detail = fmt.Sprintf("HTTP %d", resp.StatusCode)
	if resolveNote != "" {
		ch.Detail = resolveNote + "；" + ch.Detail
	}
	return ch
}

// checkForeignIPProxy 不经域名/FakeDNS，直连海外 IP:443 做 TLS，专测 geoip:!cn→代理出站。
func checkForeignIPProxy(snap StatusSnapshot) DiagnoseCheck {
	_ = snap
	start := time.Now()
	ch := DiagnoseCheck{
		ID:    "https_foreign_ip",
		Title: "外网代理路径 (1.1.1.1:443 TLS)",
		Hint:  "此项失败而 Google FakeDNS 仍过：代理出站/绑网卡有问题，或浏览器 DoH 拿到真 IP 后走不通",
	}
	d := net.Dialer{Timeout: 4 * time.Second}
	raw, err := d.DialContext(context.Background(), "tcp", "1.1.1.1:443")
	if err != nil {
		ch.OK = false
		ch.Level = "fail"
		ch.Detail = err.Error()
		ch.Elapsed = time.Since(start).Round(time.Millisecond).String()
		return ch
	}
	tlsConn := tls.Client(raw, &tls.Config{
		ServerName:         "cloudflare-dns.com",
		InsecureSkipVerify: false,
		MinVersion:         tls.VersionTLS12,
	})
	_ = tlsConn.SetDeadline(time.Now().Add(4 * time.Second))
	err = tlsConn.Handshake()
	_ = tlsConn.Close()
	ch.Elapsed = time.Since(start).Round(time.Millisecond).String()
	if err != nil {
		ch.OK = false
		ch.Level = "fail"
		ch.Detail = "TCP 通但 TLS 失败：" + err.Error()
		return ch
	}
	ch.OK = true
	ch.Level = "ok"
	ch.Detail = "TLS 握手成功（非 FakeDNS，真代理路径）"
	return ch
}

// localCNDNSTarget TUN 开启时本机 DNS 探测地址（按平台实际劫持目标）。
func localCNDNSTarget() string {
	switch runtime.GOOS {
	case "linux", "windows":
		return "127.0.0.1:53"
	default:
		// Darwin：helper 监听 :53，转发到 53535 dokodemo。
		return "127.0.0.1:53"
	}
}

// checkCNDNS TUN 开启时测本机 DNS（平台 DNS 劫持→国内解析）；关闭时仍测到 223.5.5.5 的 TCP。
func checkCNDNS(snap StatusSnapshot) DiagnoseCheck {
	start := time.Now()
	tunOn := snap.TunActive || snap.TunEnabled
	if tunOn {
		target := localCNDNSTarget()
		ch := DiagnoseCheck{
			ID:    "direct_dns",
			Title: "国内 DNS (via " + target + ")",
			Hint:  cnDNSHint(snap),
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		r := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: 2 * time.Second}
				return d.DialContext(ctx, "udp", target)
			},
		}
		ips, err := r.LookupHost(ctx, "www.baidu.com")
		ch.Elapsed = time.Since(start).Round(time.Millisecond).String()
		if err != nil {
			ch.OK = false
			ch.Level = "fail"
			ch.Detail = err.Error()
			return ch
		}
		ch.OK = true
		ch.Level = "ok"
		ch.Detail = fmt.Sprintf("解析 www.baidu.com → %v", ips)
		return ch
	}
	return checkTCP("direct_dns", "直连 DNS (223.5.5.5:53)", "223.5.5.5:53", cnDNSHint(snap))
}

func cnDNSHint(snap StatusSnapshot) string {
	if snap.TunActive || snap.TunEnabled {
		switch runtime.GOOS {
		case "darwin":
			return "开 TUN 仍失败：本机 :53 dokodemo/国内 https+local DNS 未通；可关开 TUN 或重启"
		case "linux":
			return "开 TUN 仍失败：resolv.conf 未指 127.0.0.1 或本机 :53 dokodemo/国内 tcp+local 未通；可关开 TUN 或重启"
		default:
			return "开 TUN 仍失败：本机 dokodemo/国内 tcp+local DNS 未通；可关开 TUN 或重启"
		}
	}
	return "失败时常见于 DNS 残留、未提权、或本机网络拦 223.5.5.5"
}

func cnHTTPSHint(snap StatusSnapshot) string {
	if snap.TunActive || snap.TunEnabled {
		return "开 TUN 国内不通、外网通：多为 FakeDNS 下 geosite:cn 真解析挂了"
	}
	return "国内站不通：检查本机网络/DNS/防火墙"
}

func proxyHTTPSHint(snap StatusSnapshot) string {
	if snap.TunActive || snap.TunEnabled {
		return "须能真实打开页面；仅 TCP 秒过不算。失败查代理节点/出站绑网卡/FakeDNS"
	}
	return "TUN 已关时外网直连失败通常正常；开 TUN 后再测"
}

func (h *Handler) checkRuntimeSnapshot(snap StatusSnapshot) DiagnoseCheck {
	parts := []string{
		fmt.Sprintf("os=%s elevated=%v", snap.OS, snap.Elevated),
		fmt.Sprintf("tun_use=%v active=%v if=%q owner=%q", snap.TunLocalUse, snap.TunActive, snap.TunIfName, snap.TunOwnerKey),
		fmt.Sprintf("bind=%q auto=%v", snap.TunBindIface, snap.TunBindAuto),
		fmt.Sprintf("wire=%v servers=%d/%d", snap.WireConnected, snap.HealthyCount, snap.ServerCount),
	}
	if snap.TunDegraded != "" {
		parts = append(parts, "degraded="+snap.TunDegraded)
	}
	for _, srv := range snap.Servers {
		if srv.LastError == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("err[%s]=%s", srv.Key, truncateDiag(srv.LastError, 120)))
	}
	return DiagnoseCheck{
		ID:     "snapshot",
		Title:  "运行快照",
		OK:     true,
		Level:  "ok",
		Detail: strings.Join(parts, " · "),
	}
}

func truncateDiag(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func checkOK(byID map[string]DiagnoseCheck, id string) bool {
	ch, ok := byID[id]
	return ok && ch.OK
}

func checkLevel(byID map[string]DiagnoseCheck, id string) string {
	if ch, ok := byID[id]; ok {
		return ch.Level
	}
	return ""
}

func hasFakeDNSNote(byID map[string]DiagnoseCheck, id string) bool {
	ch, ok := byID[id]
	return ok && strings.Contains(ch.Detail, "FakeDNS")
}

// synthesizeVerdict 根据检查组合给出可远程解读的根因；贴日志即可定位，不必反复上机。
func synthesizeVerdict(tunOn bool, byID map[string]DiagnoseCheck) (verdict, cause, fix string) {
	if !tunOn {
		return "TUN 未开：当前测的是直连，解释不了「开 TUN 上不了外网」",
			"tun_off",
			"开启 TUN 后再点一键排查，把整段日志发回"
	}
	if checkLevel(byID, "elevated") == "fail" {
		return "未提权：Windows 下 TUN/路由/DNS 改不动或半生效",
			"not_elevated",
			"后台「权限与自启」→ 提权重启；确认排查里「管理员权限」为通过"
	}
	if checkLevel(byID, "bind_iface") == "fail" {
		return "出站绑网卡名在本机不存在（常见于别机配置拷过来）",
			"bad_bind",
			"TUN 页清空绑网卡或改成本机真实网卡名后重开 TUN"
	}
	if checkLevel(byID, "tun") == "fail" {
		return "本地要开 TUN 但未真正生效（残留/降级/Helper）",
			"tun_inactive",
			"看「TUN 状态」明细；关开 TUN 或重启；Mac 先装 Helper"
	}
	if checkLevel(byID, "instances") == "fail" {
		return "v2ray 实例未健康运行，代理链路不存在",
			"instance_down",
			"看状态页实例 LastError；修好配置/端口后再测"
	}

	cnDNS := checkOK(byID, "direct_dns")
	cnHTTP := checkOK(byID, "https_cn")
	proxyHTTP := checkOK(byID, "https_proxy")
	foreignIP := checkOK(byID, "https_foreign_ip")
	_, hasForeign := byID["https_foreign_ip"]

	switch {
	case !cnDNS && (proxyHTTP || hasFakeDNSNote(byID, "https_proxy")):
		return "国内 DNS 上游挂了：外网靠 FakeDNS 假 IP，国内站/真域名解析失败",
			"cn_dns_upstream",
			"确认国内 DNS 上游（Darwin 默认 https+local、Linux/Windows 默认 tcp+local）；关开 TUN；看客户端标记是否最新"
	case !cnHTTP && !proxyHTTP:
		return "国内外 HTTPS 都挂：TUN 总出口或本机 DNS 全挂",
			"tun_total",
			"看运行快照 degraded/实例错误；关开 TUN 或重启"
	case !cnHTTP && proxyHTTP && (!hasForeign || foreignIP):
		return "国内 HTTPS 挂、外网通：geosite:cn 真解析/直连侧异常（FakeDNS 分流）",
			"cn_https_path",
			"查 geosite:cn 规则与国内 DNS；与「国内 DNS」项对照"
	case !proxyHTTP && hasForeign && foreignIP:
		return "海外真 IP→代理通，但 Google 域名 HTTPS 失败：FakeDNS/嗅探/域名路由问题",
			"fakedns_or_domain",
			"查 FakeDNS、sniffing、域名 routing；浏览器勿开「安全 DNS/DoH」对照"
	case proxyHTTP && hasForeign && !foreignIP:
		return "域名/FakeDNS 路径看似通，海外真 IP TLS 失败：代理出站或绑网卡出口有问题（浏览器 DoH 也易中招）",
			"proxy_ip_path",
			"查代理节点、geoip:!cn→出站、绑网卡；Chrome 关掉安全 DNS 再试网页"
	case !proxyHTTP:
		return "外网域名与海外 IP 都失败：代理整条链不通（节点/出站/路由）",
			"proxy_dead",
			"查 TUN 拥有者出站、节点是否可用、routing 默认是否进代理"
	case cnHTTP && proxyHTTP && (!hasForeign || foreignIP):
		return "本机代理自检已通过。若浏览器仍上不了外网：多半浏览器 DoH/扩展/杀软另走路径",
			"browser_side",
			"关 Chrome「使用安全 DNS」；换 Edge/无痕；临时关杀软；把本日志发回即可确认"
	default:
		return "已收集运行快照与探测项，对照 FAIL/WARN 明细即可定责",
			"mixed",
			"把完整排查日志复制发回（含「根因」与「运行快照」行）"
	}
}
