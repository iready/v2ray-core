//go:build !darwin

package tunctl

// HelperInstalledAndCurrent 非 macOS 恒为 false。
func HelperInstalledAndCurrent(helperBinary string) bool {
	_ = helperBinary
	return false
}
