//go:build darwin

package tunctl

import (
	"bytes"
	"os"
)

// HelperInstalledAndCurrent Helper 可 ping 且已安装二进制与待安装文件一致时无需重装。
func HelperInstalledAndCurrent(helperBinary string) bool {
	if !Default().HelperInstalled() {
		return false
	}
	same, err := sameFile(helperBinary, helperInstallPath)
	return err == nil && same
}

func sameFile(a, b string) (bool, error) {
	ab, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	bb, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(ab, bb), nil
}
