package mitmctl

import (
	"fmt"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

func TestApplyRestartDoesNotLeakGoroutines(t *testing.T) {
	m := &Manager{flows: NewFlowStore(32)}
	profile := rvstore.DefaultMitmProfile()
	profile.Use = true
	profile.Addr = "127.0.0.1:19095"
	profile.WebAddr = ""

	warmup := func() {
		if err := m.Apply(profile); err != nil {
			t.Fatalf("warmup apply: %v", err)
		}
		waitTCP(t, "127.0.0.1:19095", 2*time.Second)
		if err := m.Stop(); err != nil {
			t.Fatalf("warmup stop: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	warmup()
	warmup()

	runtime.GC()
	time.Sleep(20 * time.Millisecond)
	base := runtime.NumGoroutine()

	const rounds = 20
	for i := 0; i < rounds; i++ {
		if err := m.Apply(profile); err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
		waitTCP(t, "127.0.0.1:19095", 2*time.Second)
		if err := m.Stop(); err != nil {
			t.Fatalf("stop %d: %v", i, err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	var after int
	for time.Now().Before(deadline) {
		runtime.GC()
		after = runtime.NumGoroutine()
		if after <= base+5 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("goroutine leak: base=%d after %d restarts=%d (delta=%d)", base, rounds, after, after-base)
}

func TestFlowStoreRingBounded(t *testing.T) {
	s := NewFlowStore(8)
	for i := 0; i < 50; i++ {
		s.upsert(&storedFlow{sum: FlowSummary{ID: fmt.Sprintf("f%d", i), Method: "GET"}})
	}
	if s.Len() != 8 {
		t.Fatalf("len=%d want 8", s.Len())
	}
	if cap(s.seq) != 8 {
		t.Fatalf("seq cap grew: %d", cap(s.seq))
	}
	list := s.List()
	if len(list) != 8 || list[0].ID != "f49" {
		t.Fatalf("list=%+v", list)
	}
}

func waitTCP(t *testing.T, addr string, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("tcp %s not up", addr)
}
