//go:build darwin

package regService

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const launchAgentLabelPrefix = "rocket."

func launchAgentLabel(execPath string) string {
	return launchAgentLabelPrefix + filepath.Base(execPath)
}

func launchAgentPlistPath(homeDir, execPath string) string {
	return filepath.Join(homeDir, "Library", "LaunchAgents", filepath.Base(execPath)+".plist")
}

func launchAgentDomain() string {
	return "gui/" + strconv.Itoa(os.Getuid())
}

// InstallAutoStart 注册当前可执行文件为登录自启（LaunchAgent）。
func InstallAutoStart() error {
	execPath, err := filepath.Abs(os.Args[0])
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0o755); err != nil {
		return fmt.Errorf("mkdir LaunchAgents: %w", err)
	}

	baseName := filepath.Base(execPath)
	label := launchAgentLabel(execPath)
	targetFile := launchAgentPlistPath(homeDir, execPath)
	workDir := filepath.Dir(execPath)

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>ThrottleInterval</key>
    <integer>10</integer>
    <key>WorkingDirectory</key>
    <string>%s</string>
    <key>ProcessType</key>
    <string>Background</string>
    <key>StandardErrorPath</key>
    <string>/tmp/%s.err</string>
    <key>StandardOutPath</key>
    <string>/tmp/%s.out</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin</string>
    </dict>
</dict>
</plist>`, label, execPath, workDir, baseName, baseName)

	newPlist := []byte(plistContent)
	domain := launchAgentDomain()
	service := domain + "/" + label
	if old, err := os.ReadFile(targetFile); err == nil && string(old) == string(newPlist) {
		if _, err := exec.Command("launchctl", "print", service).CombinedOutput(); err == nil {
			return nil
		}
	}

	if err := os.WriteFile(targetFile, newPlist, 0o644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	_ = exec.Command("launchctl", "bootout", service).Run()
	_ = exec.Command("launchctl", "bootout", domain, targetFile).Run()
	time.Sleep(300 * time.Millisecond)

	if out, err := exec.Command("launchctl", "bootstrap", domain, targetFile).CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if !strings.Contains(msg, "already bootstrapped") {
			if _, printErr := exec.Command("launchctl", "print", service).CombinedOutput(); printErr != nil {
				return fmt.Errorf("launchctl bootstrap: %w (%s)", err, msg)
			}
		}
	}
	_ = exec.Command("launchctl", "enable", service).Run()
	if out, err := exec.Command("launchctl", "kickstart", service).CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if !strings.Contains(msg, "already running") {
			return fmt.Errorf("launchctl kickstart: %w (%s)", err, msg)
		}
	}
	return nil
}

// UninstallAutoStart 取消登录自启并删除 plist。
func UninstallAutoStart() error {
	execPath, err := filepath.Abs(os.Args[0])
	if err != nil {
		return err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	label := launchAgentLabel(execPath)
	targetFile := launchAgentPlistPath(homeDir, execPath)
	domain := launchAgentDomain()
	service := domain + "/" + label
	_ = exec.Command("launchctl", "bootout", service).Run()
	_ = exec.Command("launchctl", "bootout", domain, targetFile).Run()
	_ = exec.Command("launchctl", "disable", service).Run()
	_ = os.Remove(targetFile)
	return nil
}

// AutoStartInstalled 当前可执行文件是否已注册登录自启。
func AutoStartInstalled() bool {
	execPath, err := filepath.Abs(os.Args[0])
	if err != nil {
		return false
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	if _, err := os.Stat(launchAgentPlistPath(homeDir, execPath)); err != nil {
		return false
	}
	label := launchAgentLabel(execPath)
	_, err = exec.Command("launchctl", "print", launchAgentDomain()+"/"+label).CombinedOutput()
	return err == nil
}

// RunAutoStart macOS 由 launchctl 管理，无需手动 Run。
func RunAutoStart() error { return nil }
