//go:build windows

package regService

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows/registry"
)

const (
	autoStartTaskName = "RocketClient"
	createNoWindow    = 0x08000000
)

func execPathAbs() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(p)
}

func schtasks(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "schtasks", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd.CombinedOutput()
}

// InstallAutoStart 注册登录自启计划任务（最高权限）。创建时需已提权，之后双击普通启动也可经任务提权拉起。
func InstallAutoStart() error {
	exe, err := execPathAbs()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	tr := `"` + exe + `"`
	out, err := schtasks(
		"/Create",
		"/TN", autoStartTaskName,
		"/TR", tr,
		"/SC", "ONLOGON",
		"/RL", "HIGHEST",
		"/F",
	)
	if err != nil {
		return fmt.Errorf("schtasks create: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// UninstallAutoStart 删除登录自启计划任务。
func UninstallAutoStart() error {
	out, err := schtasks("/Delete", "/TN", autoStartTaskName, "/F")
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(strings.ToLower(msg), "cannot find") ||
			strings.Contains(msg, "不存在") ||
			strings.Contains(msg, "cannot find the file") {
			return nil
		}
		return fmt.Errorf("schtasks delete: %w (%s)", err, msg)
	}
	return nil
}

// AutoStartInstalled 是否已注册提权自启任务。
// 读注册表，避免 schtasks RPC 在 TUN/DNS 卡死时把 /api/status 一起拖死。
func AutoStartInstalled() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows NT\CurrentVersion\Schedule\TaskCache\Tree\`+autoStartTaskName,
		registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	_ = k.Close()
	return true
}

// RunAutoStart 立即运行已注册的提权任务（供非提权进程自拉起）。
func RunAutoStart() error {
	out, err := schtasks("/Run", "/TN", autoStartTaskName)
	if err != nil {
		return fmt.Errorf("schtasks run: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}
