//go:build windows

package localadmin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const createNoWindow = 0x08000000

func spawnRelaunch() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable: %w", err)
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}
	ps := fmt.Sprintf(
		`Wait-Process -Id %d -Timeout 30 -ErrorAction SilentlyContinue; Start-Sleep -Milliseconds 400; Start-Process -FilePath '%s'%s`,
		os.Getpid(),
		strings.ReplaceAll(exe, `'`, `''`),
		psArgumentList(os.Args[1:]),
	)
	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd.Start()
}

func psArgumentList(args []string) string {
	if len(args) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		quoted = append(quoted, "'"+strings.ReplaceAll(a, `'`, `''`)+"'")
	}
	return " -ArgumentList " + strings.Join(quoted, ",")
}
