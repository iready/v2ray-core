package internet_test

import (
	"context"
	"net"
	"testing"
	"time"

	v2net "github.com/v2fly/v2ray-core/v5/common/net"
	"github.com/v2fly/v2ray-core/v5/transport/internet"
)

func TestUDPPacketConnClosesWhenContextDone(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, err := internet.DialSystem(ctx, v2net.DestinationFromAddr(pc.LocalAddr()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	_ = pc.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 16)
	if _, _, err := pc.ReadFrom(buf); err != nil {
		t.Fatal(err)
	}

	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_ = conn.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
		_, err := conn.Write([]byte("x"))
		if err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("UDP socket still writable after context cancel")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
