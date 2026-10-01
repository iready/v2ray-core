// Package rocket integrates the v2fly agent with Rocket v2fly_wire.
//
// Rocket config_gen.py emits v2fly JSON using these features (client must support all):
//   - inbounds/outbounds/routing/dns (standard v2fly v5)
//   - reverse bridges/portals (app/reverse)
//   - loopback outbound + dokodemo-door (port mapping)
//   - services.tun via tun_config.py（sing-tun；分流由 routing.rules 配置）
//   - geoip / geosite routing (embedded geo in main/z/res)
//   - multiple server keys per token (one core.Instance per key)
//
// Not used by the client today: api_conf (legacy commander gRPC port).
package rocket
