// Package notify 跨平台桌面通知（仅 Windows / macOS；失败仅打日志，不阻断业务）。
package notify

import "log"

// Info 弹出桌面通知；非桌面平台直接跳过。
func Info(title, body string) {
	if !desktopEnabled() {
		return
	}
	if title == "" {
		title = "Rocket"
	}
	if err := show(title, body); err != nil {
		log.Printf("桌面通知失败: %v (%s: %s)", err, title, body)
		return
	}
	log.Printf("桌面通知: %s — %s", title, body)
}
