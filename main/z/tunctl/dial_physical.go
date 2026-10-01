package tunctl

import (
	"context"
	"net"
	"time"
)

// DialPhysicalTCP 绑物理网卡打 TCP，域名走 223.5.5.5，不经 TUN / 系统 127.0.0.1 DNS。
func DialPhysicalTCP(ctx context.Context, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if net.ParseIP(host) == nil {
		ip, err := resolveCNHost(ctx, host)
		if err != nil {
			return nil, err
		}
		if ip == "" {
			return nil, &net.DNSError{Err: "no A record", Name: host, IsNotFound: true}
		}
		host = ip
	}
	timeout := 2 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		if remain := time.Until(deadline); remain > 0 && remain < timeout {
			timeout = remain
		}
	}
	d := net.Dialer{Timeout: timeout, Control: bindPhysicalControl}
	return d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
}
