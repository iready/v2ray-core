package regService

import "os"

// InContainer 是否在容器内运行（无 systemd 托管）。
func InContainer() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}
