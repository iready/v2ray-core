package rocket

import (
	"testing"

	pb "github.com/v2fly/v2ray-core/v5/main/z/proto"
)

func TestRunningInstance(t *testing.T) {
	if _, err := runningInstance(nil, "a"); err == nil || err.Error() != "实例未运行" {
		t.Fatalf("nil rs: %v", err)
	}
	rs := &RS{Servers: map[string]*ServerInstance{}}
	if _, err := runningInstance(rs, "a"); err == nil || err.Error() != "实例未运行" {
		t.Fatalf("missing: %v", err)
	}
	rs.Servers["a"] = &ServerInstance{Key: "a", Status: pb.ServerStatus_READY}
	if _, err := runningInstance(rs, "a"); err == nil || err.Error() != "实例未运行" {
		t.Fatalf("stopped: %v", err)
	}
}
