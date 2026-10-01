package singtun

import (
	"net"
	"testing"
)

type handshakeConn struct {
	net.Conn
	called bool
	err    error
}

func (c *handshakeConn) HandshakeSuccess() error {
	c.called = true
	return c.err
}

func TestAcceptTUNHandshake(t *testing.T) {
	c := &handshakeConn{}
	if err := acceptTUNHandshake(c); err != nil {
		t.Fatal(err)
	}
	if !c.called {
		t.Fatal("expected handshake before outbound dial")
	}
	if err := acceptTUNHandshake(&struct{ net.Conn }{}); err != nil {
		t.Fatal(err)
	}
}
