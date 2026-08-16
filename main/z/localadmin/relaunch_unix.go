//go:build !windows

package localadmin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func spawnRelaunch() error {
	if os.Getenv("INVOCATION_ID") != "" {
		// systemd Restart=always：只退出，由 unit 拉起，避免双进程。
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}
	parts := make([]string, 0, 1+len(os.Args[1:]))
	parts = append(parts, shellQuote(exe))
	for _, a := range os.Args[1:] {
		parts = append(parts, shellQuote(a))
	}
	script := fmt.Sprintf(
		"while kill -0 %s 2>/dev/null; do sleep 0.2; done; sleep 0.4; exec %s",
		strconv.Itoa(os.Getpid()),
		strings.Join(parts, " "),
	)
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, `'`, `'"'"'`) + "'"
}
