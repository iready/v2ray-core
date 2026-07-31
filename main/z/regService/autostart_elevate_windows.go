//go:build windows

package regService

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func runElevatedWait(arg string) error {
	exe, err := execPathAbs()
	if err != nil {
		return err
	}
	ps := fmt.Sprintf(
		`Start-Process -FilePath '%s' -ArgumentList '%s' -Verb RunAs -Wait`,
		strings.ReplaceAll(exe, `'`, `''`),
		arg,
	)
	cmd := exec.Command("powershell", "-NoProfile", "-WindowStyle", "Hidden", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("UAC %s: %w (%s)", arg, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// InstallAutoStartWithUAC 当前未提权时弹出 UAC，用提权子进程完成 -install-autostart。
func InstallAutoStartWithUAC() error {
	if err := runElevatedWait("-install-autostart"); err != nil {
		return err
	}
	if !AutoStartInstalled() {
		return fmt.Errorf("未完成注册（可能取消了 UAC）")
	}
	return nil
}

// UninstallAutoStartWithUAC 未提权时经 UAC 删除计划任务。
func UninstallAutoStartWithUAC() error {
	if err := runElevatedWait("-uninstall-autostart"); err != nil {
		return err
	}
	if AutoStartInstalled() {
		return fmt.Errorf("未完成卸载（可能取消了 UAC）")
	}
	return nil
}
