package singtun

import (
	"context"
	"time"

	stun "github.com/sagernet/sing-tun"
)

const (
	icmpTimeout = 5 * time.Second
	// UDP NAT 空闲超时；过长会在 Windows 全流量劫持下堆积大量出站 ListenPacket。
	udpTimeout = 30 * time.Second
)

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
			return nil, err
		}
		opts.InterfaceFinder = CurrentInterfaceFinder()
	}
	tunIf, err := stun.New(opts)
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
		_ = tunIf.Close()
		detachInterfaceMonitors()
		return nil, err
	}
	if err := tunIf.Start(); err != nil {
		_ = tunStack.Close()
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
