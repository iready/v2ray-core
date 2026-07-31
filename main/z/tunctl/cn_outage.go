package tunctl

import (
	"context"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// 国内主流 HTTPS，用来区分「控制面掉了」和「整机没网」。
var cnOutageTargets = []string{
	"www.baidu.com:443",
	"www.qq.com:443",
	"www.taobao.com:443",
}

const (
	cnOutageDNS     = "223.5.5.5:53"
	cnOutageTimeout = 1500 * time.Millisecond
	cnOutageMinFail = 2
)

// DomesticMassOutage 并行探测国内主流站；至少 2 个失败才算大面积断网。
// 解析走 223.5.5.5、出站绑物理网卡，不经 TUN / 系统 127.0.0.1 DNS。
func DomesticMassOutage() bool {
	n := len(cnOutageTargets)
	var okN atomic.Int32
	var wg sync.WaitGroup
	for _, target := range cnOutageTargets {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			if probeCNTarget(target) {
				okN.Add(1)
			}
		}(target)
	}
	wg.Wait()
	ok := int(okN.Load())
	fail := n - ok
	if fail >= cnOutageMinFail {
		log.Printf("国内连通性 %d/%d 失败，视为大面积断网", fail, n)
		return true
	}
	return false
}

func domesticOutageByCounts(ok, n int) bool {
	return n-ok >= cnOutageMinFail
}

func probeCNTarget(target string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), cnOutageTimeout)
	defer cancel()
	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return false
	}
	ip, err := resolveCNHost(ctx, host)
	if err != nil || ip == "" {
		return false
	}
	d := net.Dialer{Timeout: cnOutageTimeout, Control: bindPhysicalControl}
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, port))
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func resolveCNHost(ctx context.Context, host string) (string, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: cnOutageTimeout, Control: bindPhysicalControl}
			return d.DialContext(ctx, network, cnOutageDNS)
		},
	}
	ips, err := r.LookupIP(ctx, "ip4", host)
	if err != nil || len(ips) == 0 {
		return "", err
	}
	return ips[0].String(), nil
}

func bindPhysicalControl(network, address string, c syscall.RawConn) error {
	iface := getDialBindIface()
	if iface == "" {
		return nil
	}
	var bindErr error
	err := c.Control(func(fd uintptr) {
		bindErr = bindOutboundToInterface(network, address, fd, iface)
	})
	if err != nil {
		return err
	}
	return bindErr
}
