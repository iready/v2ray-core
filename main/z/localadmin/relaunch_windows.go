//go:build windows

package localadmin

import (
	"golang.org/x/sys/windows"
	"strings"
	"syscall"
)

const (
	createNoWindow         = 0x08000000
	createNewProcessGroup  = 0x00000200
	createBreakawayFromJob = 0x01000000
)

// Elevation uses the same wait-for-old-PID handoff, rather than racing a no-argument task.
func spawnElevatedRelaunch() error {
	cmd, err := newRelaunchCmd()
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	exe, err := windows.UTF16PtrFromString(cmd.Path)
	if err != nil {
		return err
	}
	args := make([]string, 0, len(cmd.Args)-1)
	for _, arg := range cmd.Args[1:] {
		args = append(args, syscall.EscapeArg(arg))
	}
	params, err := windows.UTF16PtrFromString(strings.Join(args, " "))
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, exe, params, nil, 0)
}

func spawnRelaunch() error {
	cmd, err := newRelaunchCmd()
	if err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup | createBreakawayFromJob,
	}
	if err := cmd.Start(); err == nil {
		return nil
	}
	// 当前 Job 不允许 breakaway 时 CreateProcess 会失败，去掉该旗再拉一次。
	cmd, err = newRelaunchCmd()
	if err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup,
	}
	return cmd.Start()
}
