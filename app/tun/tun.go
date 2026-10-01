//go:build !confonly
// +build !confonly

package tun

import (
	"context"

	core "github.com/v2fly/v2ray-core/v5"
	"github.com/v2fly/v2ray-core/v5/app/tun/singtun"
	"github.com/v2fly/v2ray-core/v5/common"
	"github.com/v2fly/v2ray-core/v5/features/outbound"
	"github.com/v2fly/v2ray-core/v5/features/policy"
	"github.com/v2fly/v2ray-core/v5/features/routing"
)

//go:generate go run github.com/v2fly/v2ray-core/v5/common/errors/errorgen

type TUN struct {
	ctx           context.Context
	dispatcher    routing.Dispatcher
	router        routing.Router
	outbound      outbound.Manager
	policyManager policy.Manager
	config        *Config
	engine        *singtun.Engine
}

func (t *TUN) Type() interface{} {
	return (*TUN)(nil)
}

func (t *TUN) Start() error {
	input := singtun.ConfigInput{
		Name:             t.config.Name,
		MTU:              t.config.Mtu,
		Tag:              t.config.Tag,
		UserLevel:        t.config.UserLevel,
		IPs:              t.config.Ips,
		Routes:           t.config.Routes,
		SniffingSettings: t.config.SniffingSettings,
	}
	opts, stackName, input, exclude, bypassHosts := singtun.BuildOptions(input)
	tunAddrs := singtun.TunInterfaceAddrs(opts.Inet4Address, opts.Inet6Address)
	handler := singtun.NewHandler(t.ctx, t.dispatcher, t.router, t.outbound, t.policyManager, input, exclude, bypassHosts, tunAddrs)
	engine, err := singtun.StartWithOptions(t.ctx, opts, stackName, handler)
	if err != nil {
		return newError("failed to start sing-tun").Base(err).AtError()
	}
	t.engine = engine
	return nil
}

func (t *TUN) Close() error {
	if t.engine != nil {
		err := t.engine.Close()
		t.engine = nil
		singtun.ResetPlatform()
		return err
	}
	return nil
}

func (t *TUN) Init(ctx context.Context, config *Config, dispatcher routing.Dispatcher, router routing.Router, outboundManager outbound.Manager, policyManager policy.Manager) error {
	t.ctx = ctx
	t.config = config
	t.dispatcher = dispatcher
	t.router = router
	t.outbound = outboundManager
	t.policyManager = policyManager
	return nil
}

func init() {
	common.Must(common.RegisterConfig((*Config)(nil), func(ctx context.Context, config interface{}) (interface{}, error) {
		tun := new(TUN)
		err := core.RequireFeatures(ctx, func(d routing.Dispatcher, r routing.Router, o outbound.Manager, p policy.Manager) error {
			return tun.Init(ctx, config.(*Config), d, r, o, p)
		})
		return tun, err
	}))
}
