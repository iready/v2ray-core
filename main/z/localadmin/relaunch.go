package localadmin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const relaunchWaitFlag = "-relaunch-wait"

func newRelaunchCmd() (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("executable: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return nil, err
	}
	args := append([]string{relaunchWaitFlag, strconv.Itoa(os.Getpid())}, stripRelaunchWait(os.Args[1:])...)
	cmd := exec.Command(exe, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd, nil
}

func stripRelaunchWait(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == relaunchWaitFlag {
			if i+1 < len(args) {
				i++
			}
			continue
		}
		if strings.HasPrefix(a, relaunchWaitFlag+"=") {
			continue
		}
		out = append(out, a)
	}
	return out
}

// WaitRelaunch 等旧进程退出后再继续，避免端口/runtime 冲突。
func WaitRelaunch(pid int) {
	if pid <= 0 {
		return
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		alive, err := ProcessAlive(pid)
		if err != nil || !alive {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(400 * time.Millisecond)
}
