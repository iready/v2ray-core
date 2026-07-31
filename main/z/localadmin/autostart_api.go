package localadmin

import (
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/v2fly/v2ray-core/v5/main/z/regService"
	"github.com/v2fly/v2ray-core/v5/main/z/tunctl"
)

type AutostartStatus struct {
	Elevated  bool   `json:"elevated"`
	Installed bool   `json:"installed"`
	Message   string `json:"message,omitempty"`
}

func (h *Handler) GetAutostart(c *gin.Context) {
	c.JSON(http.StatusOK, AutostartStatus{
		Elevated:  tunctl.IsElevated(),
		Installed: regService.AutoStartInstalled(),
	})
}

func (h *Handler) PostAutostartInstall(c *gin.Context) {
	var err error
	if tunctl.IsElevated() {
		err = regService.InstallAutoStart()
	} else {
		err = regService.InstallAutoStartWithUAC()
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, AutostartStatus{
		Elevated:  tunctl.IsElevated(),
		Installed: true,
		Message:   "已开启登录提权自启；未提权时可点「立即提权重启」",
	})
}

func (h *Handler) PostAutostartUninstall(c *gin.Context) {
	var err error
	if tunctl.IsElevated() {
		err = regService.UninstallAutoStart()
	} else {
		err = regService.UninstallAutoStartWithUAC()
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, AutostartStatus{
		Elevated:  tunctl.IsElevated(),
		Installed: false,
		Message:   "已关闭提权自启",
	})
}

func (h *Handler) PostAutostartRelaunch(c *gin.Context) {
	if tunctl.IsElevated() {
		c.JSON(http.StatusOK, AutostartStatus{
			Elevated:  true,
			Installed: regService.AutoStartInstalled(),
			Message:   "当前已是管理员权限",
		})
		return
	}
	if !regService.AutoStartInstalled() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请先开启提权自启"})
		return
	}
	if err := regService.RunAutoStart(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, AutostartStatus{
		Elevated:  false,
		Installed: true,
		Message:   "正在提权重启，本窗口即将关闭",
	})
	go func() {
		time.Sleep(400 * time.Millisecond)
		os.Exit(0)
	}()
}
