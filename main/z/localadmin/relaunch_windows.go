//go:build windows

package localadmin

import "syscall"

const (
	createNoWindow         = 0x08000000
	createNewProcessGroup  = 0x00000200
	createBreakawayFromJob = 0x01000000
)

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
