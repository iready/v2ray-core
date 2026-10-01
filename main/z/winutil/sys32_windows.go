package winutil

import (
	"os"
	"path/filepath"
	"strings"
)

// System32 返回 SystemRoot\System32 下的 exe，不查 PATH。
func System32(name string) string {
	root := strings.TrimSpace(os.Getenv("SystemRoot"))
	if root == "" {
		root = `C:\Windows`
	}
	base := strings.TrimSuffix(name, ".exe")
	base = strings.TrimSuffix(base, ".EXE")
	if strings.EqualFold(base, "powershell") {
		return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	}
	return filepath.Join(root, "System32", base+".exe")
}
