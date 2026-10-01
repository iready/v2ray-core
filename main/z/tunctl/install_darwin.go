//go:build darwin

package tunctl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/v2fly/v2ray-core/v5/main/z/helper"
)

const (
	helperInstallPath = "/Library/PrivilegedHelperTools/com.v2ray.helper"
	launchDaemonPath  = "/Library/LaunchDaemons/com.v2ray.helper.plist"
)

func helperNonInteractive() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ROCKET_HELPER_NONINTERACTIVE")))
	return v == "1" || v == "true" || v == "yes"
}

func privilegedScriptPath() string {
	if p := strings.TrimSpace(os.Getenv("ROCKET_HELPER_PRIV_SCRIPT")); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	try := filepath.Join(filepath.Dir(exe), "..", ".cursor", "hooks", "helper-privileged.sh")
	abs, err := filepath.Abs(try)
	if err != nil {
		return ""
	}
	if st, err := os.Stat(abs); err == nil && !st.IsDir() {
		return abs
	}
	return ""
}

// InstallHelper copies the helper binary and writes the embedded launchd plist, then loads the daemon.
func InstallHelper(helperBinary string) error {
	if HelperInstalledAndCurrent(helperBinary) {
		return nil
	}
	plistPath, cleanup, err := materializeHelperPlist()
	if err != nil {
		return err
	}
	defer cleanup()
	uid := os.Getuid()
	if script := privilegedScriptPath(); script != "" {
		if out, err := exec.Command("sudo", "-n", script, "install", helperBinary, plistPath, strconv.Itoa(uid)).CombinedOutput(); err == nil {
			return nil
		} else if helperNonInteractive() {
			return fmt.Errorf("install helper (sudo -n): %w (%s)", err, strings.TrimSpace(string(out)))
		}
	}
	shell := darwinHelperInstallShell(uid, shellQuote(helperBinary), shellQuote(plistPath))
	if out, err := runPrivilegedShell(shell); err == nil {
		_ = out
		return nil
	} else if helperNonInteractive() {
		return fmt.Errorf("install helper: %w", err)
	}
	script := fmt.Sprintf(`do shell script "%s" with administrator privileges`,
		escapeAppleScript(shell),
	)
	cmd := exec.Command("osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install helper: %w (%s)", err, string(out))
	}
	return nil
}

func materializeHelperPlist() (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "com.v2ray.helper-*.plist")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp plist: %w", err)
	}
	path = f.Name()
	cleanup = func() { _ = os.Remove(path) }
	if _, err := f.Write(helper.LaunchDaemonPlist); err != nil {
		_ = f.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("write temp plist: %w", err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close temp plist: %w", err)
	}
	return path, cleanup, nil
}

// UninstallHelper 停止 launchd 服务、清理路由/文件，并移除 Privileged Helper。
func UninstallHelper() error {
	_ = Default().Cleanup()
	if script := privilegedScriptPath(); script != "" {
		if out, err := exec.Command("sudo", "-n", script, "uninstall").CombinedOutput(); err == nil {
			_ = out
			return nil
		} else if helperNonInteractive() {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf("uninstall helper (sudo -n): %s", msg)
		}
	}
	out, err := runPrivilegedShell(darwinHelperUninstallShell())
	if err == nil {
		return nil
	}
	if helperNonInteractive() {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("uninstall helper: %s", msg)
	}
	script := fmt.Sprintf(`do shell script "%s" with administrator privileges`,
		escapeAppleScript(darwinHelperUninstallShell()),
	)
	out, err = exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("uninstall helper: %s", msg)
	}
	return nil
}

func runPrivilegedShell(shell string) ([]byte, error) {
	out, err := exec.Command("sudo", "-n", "sh", "-c", shell).CombinedOutput()
	if err == nil {
		return out, nil
	}
	if helperNonInteractive() {
		return out, err
	}
	script := fmt.Sprintf(`do shell script "%s" with administrator privileges`,
		escapeAppleScript(shell),
	)
	return exec.Command("osascript", "-e", script).CombinedOutput()
}

func runAdminShell(shell string) ([]byte, error) {
	return runPrivilegedShell(shell)
}

func darwinHelperInstallShell(uid int, helperBinary, plistPath string) string {
	return strings.Join([]string{
		"set -e",
		fmt.Sprintf("LABEL=%s", helper.Label),
		fmt.Sprintf("PLIST=%s", launchDaemonPath),
		fmt.Sprintf("BIN=%s", helperInstallPath),
		"mkdir -p /Library/PrivilegedHelperTools /Library/LaunchDaemons /var/run",
		fmt.Sprintf("cp %s \"$BIN\" && chmod 755 \"$BIN\"", helperBinary),
		fmt.Sprintf("cp %s \"$PLIST\"", plistPath),
		fmt.Sprintf("echo %d > %s && chmod 644 %s", uid, helper.AllowedUIDPersistPath, helper.AllowedUIDPersistPath),
		fmt.Sprintf("cp %s /var/run/com.v2ray.helper.uid && chmod 644 /var/run/com.v2ray.helper.uid", helper.AllowedUIDPersistPath),
		"launchctl bootout system/$LABEL 2>/dev/null || true",
		"launchctl bootout system \"$PLIST\" 2>/dev/null || true",
		"launchctl unload \"$PLIST\" 2>/dev/null || true",
		"launchctl enable system/$LABEL 2>/dev/null || true",
		"launchctl bootstrap system \"$PLIST\" 2>/dev/null || launchctl load \"$PLIST\"",
		"launchctl kickstart system/$LABEL 2>/dev/null || true",
	}, "; ")
}

func darwinHelperUninstallShell() string {
	return strings.Join([]string{
		fmt.Sprintf("LABEL=%s", helper.Label),
		fmt.Sprintf("PLIST=%s", launchDaemonPath),
		fmt.Sprintf("BIN=%s", helperInstallPath),
		"launchctl bootout system/$LABEL 2>/dev/null || true",
		"launchctl bootout system \"$PLIST\" 2>/dev/null || true",
		"launchctl unload \"$PLIST\" 2>/dev/null || true",
		"launchctl remove $LABEL 2>/dev/null || true",
		"launchctl disable system/$LABEL 2>/dev/null || true",
		"PID=$(pgrep -x com.v2ray.helper 2>/dev/null || true)",
		"if [ -n \"$PID\" ]; then kill -TERM \"$PID\" 2>/dev/null || true; sleep 1; kill -KILL \"$PID\" 2>/dev/null || true; fi",
		"rm -f \"$PLIST\" \"$BIN\"",
		fmt.Sprintf("rm -f /var/run/com.v2ray.helper.sock /var/run/com.v2ray.helper.uid %s", helper.AllowedUIDPersistPath),
		"rm -f /var/log/com.v2ray.helper.log /var/log/com.v2ray.helper.err.log",
		"rm -f /var/run/com.v2ray.helper.fd.*.sock 2>/dev/null || true",
	}, "; ")
}

func escapeAppleScript(shell string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(shell)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\'\'`) + "'"
}

// HelperBinaryPath returns the default path next to the client binary.
func HelperBinaryPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "v2ray-helper"
	}
	return filepath.Join(filepath.Dir(exe), "v2ray-helper")
}
