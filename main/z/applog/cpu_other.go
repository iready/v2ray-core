//go:build !windows

package applog

import "time"

// No synthetic CPU percentage when process CPU accounting is unavailable.
func ProcessCPU() (time.Duration, bool) { return 0, false }
