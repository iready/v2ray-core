//go:build !windows

package localadmin

import (
	"os"
	"syscall"
)

// ProcessAlive 判断 pid 是否仍在运行。
func ProcessAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false, err
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true, nil
	}
	return false, nil
}
