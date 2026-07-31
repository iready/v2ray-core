//go:build windows

package localadmin

import (
	"golang.org/x/sys/windows"
)

const stillActive = 259

// ProcessAlive 判断 pid 是否仍在运行（Windows 不能用 Signal(0)）。
func ProcessAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false, nil
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false, err
	}
	return code == stillActive, nil
}
