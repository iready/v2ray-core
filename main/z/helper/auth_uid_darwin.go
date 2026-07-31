//go:build darwin

package helper

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// EnsureAllowedUIDFile 在 Helper 启动时同步 uid 文件（/var/run 重启后会清空）。
func EnsureAllowedUIDFile() error {
	persist, persistErr := os.ReadFile(AllowedUIDPersistPath)
	run, runErr := os.ReadFile(AllowedUIDPath)
	switch {
	case persistErr == nil && runErr != nil:
		return writeAllowedUIDFile(AllowedUIDPath, persist)
	case runErr == nil && persistErr != nil:
		if err := writeAllowedUIDFile(AllowedUIDPersistPath, run); err != nil {
			return err
		}
		return writeAllowedUIDFile(AllowedUIDPath, run)
	case persistErr != nil && runErr != nil:
		return fmt.Errorf("allowed uid not configured (reinstall helper)")
	default:
		if strings.TrimSpace(string(persist)) != strings.TrimSpace(string(run)) {
			return writeAllowedUIDFile(AllowedUIDPath, persist)
		}
		return nil
	}
}

func writeAllowedUIDFile(path string, data []byte) error {
	uid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || uid <= 0 {
		return fmt.Errorf("invalid allowed uid in %s", path)
	}
	line := fmt.Sprintf("%d\n", uid)
	if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
		return err
	}
	return nil
}
