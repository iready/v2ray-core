package localadmin

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/v2fly/v2ray-core/v5/main/z/rvstore"
)

// ExistingInstance 若已有存活实例占用本地后台，返回其 PID/端口。
// 判定：runtime.pid 存活，且 AdminPort（或 preferred）可 TCP 连通。
func ExistingInstance(rt rvstore.Runtime, preferredPort int) (pid, port int, ok bool) {
	pid = rt.PID
	if pid <= 0 || pid == os.Getpid() {
		return 0, 0, false
	}
	alive, err := ProcessAlive(pid)
	if err != nil || !alive {
		return 0, 0, false
	}
	port = rt.AdminPort
	if port <= 0 {
		port = preferredPort
	}
	if port <= 0 {
		return 0, 0, false
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 400*time.Millisecond)
	if err != nil {
		return 0, 0, false
	}
	_ = conn.Close()
	return pid, port, true
}
