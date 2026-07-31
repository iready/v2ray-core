package tunctl

// SuspendRoutes 暂时移除 TUN split route（保留 utun 与 bypass）。
// 仅应包住 Wire dial 窗口；失败退避期间必须 Resume，否则流量直出像断网。
func SuspendRoutes() error {
	return defaultManager.SuspendRoutes()
}

// ResumeRoutes 恢复 TUN split route。
func ResumeRoutes() error {
	return defaultManager.ResumeRoutes()
}
