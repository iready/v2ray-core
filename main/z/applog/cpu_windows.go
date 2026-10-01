//go:build windows

package applog

import (
	"golang.org/x/sys/windows"
	"time"
)

// ProcessCPU returns kernel+user process time, not wall time in blocked syscalls.
func ProcessCPU() (time.Duration, bool) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		return 0, false
	}
	ticks := (uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)) + (uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime))
	return time.Duration(ticks * 100), true
}
