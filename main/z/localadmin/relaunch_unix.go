//go:build !windows

package localadmin

import (
	"os"
	"syscall"
)

func spawnRelaunch() error {
	if os.Getenv("INVOCATION_ID") != "" {
		return nil
	}
	cmd, err := newRelaunchCmd()
	if err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
