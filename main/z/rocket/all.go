package rocket

import (
	// Mandatory features. Can't remove unless there are replacements.
	_ "github.com/v2fly/v2ray-core/v5/app/dispatcher"
	_ "github.com/v2fly/v2ray-core/v5/app/proxyman/inbound"
	_ "github.com/v2fly/v2ray-core/v5/app/proxyman/outbound"

	// Default commander and all its services. This is an optional feature.
	_ "github.com/v2fly/v2ray-core/v5/app/commander"
	_ "github.com/v2fly/v2ray-core/v5/app/log/command"
	_ "github.com/v2fly/v2ray-core/v5/app/proxyman/command"
	_ "github.com/v2fly/v2ray-core/v5/app/stats/command"
	// Other optional features.
	_ "github.com/v2fly/v2ray-core/v5/app/dns"
	_ "github.com/v2fly/v2ray-core/v5/app/dns/fakedns"
	_ "github.com/v2fly/v2ray-core/v5/app/log"
	_ "github.com/v2fly/v2ray-core/v5/app/policy"
	_ "github.com/v2fly/v2ray-core/v5/app/reverse"
	_ "github.com/v2fly/v2ray-core/v5/app/router"
	_ "github.com/v2fly/v2ray-core/v5/app/stats"

	// Inbound and outbound proxies.
	_ "github.com/v2fly/v2ray-core/v5/proxy/blackhole"
	_ "github.com/v2fly/v2ray-core/v5/proxy/dns"
	_ "github.com/v2fly/v2ray-core/v5/proxy/dokodemo"
	_ "github.com/v2fly/v2ray-core/v5/proxy/freedom"
	_ "github.com/v2fly/v2ray-core/v5/proxy/http"
	_ "github.com/v2fly/v2ray-core/v5/proxy/socks"
	_ "github.com/v2fly/v2ray-core/v5/proxy/vless/inbound"
	_ "github.com/v2fly/v2ray-core/v5/proxy/vless/outbound"
	_ "github.com/v2fly/v2ray-core/v5/proxy/vmess/inbound"
	_ "github.com/v2fly/v2ray-core/v5/proxy/vmess/outbound"
	// Transports
	_ "github.com/v2fly/v2ray-core/v5/transport/internet/domainsocket"
	_ "github.com/v2fly/v2ray-core/v5/transport/internet/grpc"
	_ "github.com/v2fly/v2ray-core/v5/transport/internet/http"
	_ "github.com/v2fly/v2ray-core/v5/transport/internet/kcp"
	_ "github.com/v2fly/v2ray-core/v5/transport/internet/quic"
	_ "github.com/v2fly/v2ray-core/v5/transport/internet/tcp"
	_ "github.com/v2fly/v2ray-core/v5/transport/internet/tls"
	_ "github.com/v2fly/v2ray-core/v5/transport/internet/tls/utls"
	_ "github.com/v2fly/v2ray-core/v5/transport/internet/udp"
	_ "github.com/v2fly/v2ray-core/v5/transport/internet/websocket"

	// Geo loaders
	_ "github.com/v2fly/v2ray-core/v5/infra/conf/geodata/memconservative"

	// JSON, TOML, YAML config support. (jsonv4) This disable selective compile
	_ "github.com/v2fly/v2ray-core/v5/main/formats"

	// Commands that rely on jsonv4 format This disable selective compile
	_ "github.com/v2fly/v2ray-core/v5/main/commands/all/api/jsonv4"
	_ "github.com/v2fly/v2ray-core/v5/main/commands/all/jsonv4"

	// V5 version of json configure file parser
	_ "github.com/v2fly/v2ray-core/v5/infra/conf/v5cfg"

	_ "github.com/v2fly/v2ray-core/v5/main/z/res"
)
