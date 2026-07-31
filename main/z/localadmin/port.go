package localadmin

import (
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

const defaultPortAttempts = 5

// AllocateAdminPort 从 preferred 起尝试绑定 TCP 端口，失败则 port+1，最多 attempts 次。
func AllocateAdminPort(host string, preferred, attempts int) (port int, listener net.Listener, err error) {
	if attempts <= 0 {
		attempts = defaultPortAttempts
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		port = preferred + i
		addr := fmt.Sprintf("%s:%d", host, port)
		ln, listenErr := net.Listen("tcp", addr)
		if listenErr == nil {
			return port, ln, nil
		}
		lastErr = listenErr
	}
	return 0, nil, fmt.Errorf("bind admin port [%d,%d): %w", preferred, preferred+attempts, lastErr)
}

// KillStaleProcess 结束旧 agent 进程（仅当后台端口被占用时调用）。
func KillStaleProcess(pid int) {
	if pid <= 0 || pid == os.Getpid() {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	_ = proc.Signal(syscall.SIGTERM)
	time.Sleep(2 * time.Second)
	if alive, _ := processAlive(pid); alive {
		_ = proc.Kill()
		time.Sleep(500 * time.Millisecond)
	}
}

func processAlive(pid int) (bool, error) {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, err
	}
	return proc.Signal(syscall.Signal(0)) == nil, nil
}
