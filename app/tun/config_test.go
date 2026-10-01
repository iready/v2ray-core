package tun

import (
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
)

func TestUDPBridgeConfigJSON(t *testing.T) {
	var config Config
	if err := protojson.Unmarshal([]byte(`{
		"name": "packetbridge",
		"mtu": 1500,
		"udp_bridge": {
			"listen_address": "127.0.0.1",
			"listen_port": 9090,
			"peer_address": "127.0.0.1",
			"peer_port": 9091,
			"queue_size": 512
		}
	}`), &config); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if config.UdpBridge == nil {
		t.Fatal("udp_bridge was not decoded")
	}
	if config.UdpBridge.ListenPort != 9090 || config.UdpBridge.PeerPort != 9091 {
		t.Fatalf("unexpected bridge ports: %+v", config.UdpBridge)
	}
}

func TestPacketEncodingBypassPortsJSON(t *testing.T) {
	var config Config
	if err := protojson.Unmarshal([]byte(`{
		"packet_encoding": "Stream",
		"packet_encoding_bypass_ports": [53, 123]
	}`), &config); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if got := config.PacketEncodingBypassPorts; len(got) != 2 || got[0] != 53 || got[1] != 123 {
		t.Fatalf("packet_encoding_bypass_ports = %v, want [53 123]", got)
	}
}
