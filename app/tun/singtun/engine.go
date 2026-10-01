package singtun

import (
	"context"
	"log"
	"strings"
	"sync/atomic"
	"time"

	stun "github.com/sagernet/sing-tun"
)

var liveUDP atomic.Int32
var LiveTCP atomic.Int32
var UploadBytes, DownloadBytes atomic.Uint64

const (
	icmpTimeout = 5 * time.Second
	// 长连接（微信、QUIC）要扛过静默心跳。DNS/NTP 在 handleUDP 里单独收到 5 秒，避免再把 NAT 槽打满。
	udpTimeout = 3 * time.Minute
	udpOneShot = 5 * time.Second
)

// LiveUDP 当前还没收掉的 TUN UDP 会话数。
func LiveUDP() int { return int(liveUDP.Load()) }

type Engine struct {
	tunIf    stun.Tun
	tunStack stun.Stack
}

func StartWithOptions(ctx context.Context, opts stun.Options, stackName string, handler *Handler) (*Engine, error) {
	if err := attachInterfaceMonitors(&opts); err != nil {
		return nil, err
	}
	if opts.InterfaceFinder == nil {
		if err := EnsureStandaloneMonitor(); err != nil {
			detachInterfaceMonitors()
			return nil, err
		}
		opts.InterfaceFinder = CurrentInterfaceFinder()
	}
	at := time.Now()
	tunIf, err := stun.New(opts)
	// Only retry the known Windows adapter teardown race, never permissions/config errors.
	for attempt := 1; err != nil && attempt < 4 && strings.Contains(err.Error(), "create adapter") && strings.Contains(err.Error(), "open existing adapter"); attempt++ {
		log.Printf("phase=tun_adapter retry=%d err=%v", attempt, err)
		select {
		case <-ctx.Done():
			detachInterfaceMonitors()
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 250 * time.Millisecond):
		}
		tunIf, err = stun.New(opts)
	}
	log.Printf("phase=tun_adapter elapsed=%s err=%v", time.Since(at), err)
	if err != nil {
		detachInterfaceMonitors()
		return nil, err
	}

	tunStack, err := stun.NewStack(stackName, stun.StackOptions{
		Context:                ctx,
		Tun:                    tunIf,
		TunOptions:             opts,
		UDPTimeout:             udpTimeout,
		ICMPTimeout:            icmpTimeout,
		Handler:                handler,
		Logger:                 nopLogger{},
		InterfaceFinder:        opts.InterfaceFinder,
		ForwarderBindInterface: opts.InterfaceFinder != nil,
	})
	if err != nil {
		_ = tunIf.Close()
		detachInterfaceMonitors()
		return nil, err
	}

	if err := tunStack.Start(); err != nil {
		_ = tunStack.Close()
		_ = tunIf.Close()
		detachInterfaceMonitors()
		return nil, err
	}
	if err := tunIf.Start(); err != nil {
		_ = tunStack.Close()
		_ = tunIf.Close()
		detachInterfaceMonitors()
		return nil, err
	}

	return &Engine{tunIf: tunIf, tunStack: tunStack}, nil
}

func (e *Engine) Close() error {
	if e == nil {
		return nil
	}
	defer detachInterfaceMonitors()
	var first error
	if e.tunStack != nil {
		if err := e.tunStack.Close(); err != nil && first == nil {
			first = err
		}
	}
	if e.tunIf != nil {
		if err := e.tunIf.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
