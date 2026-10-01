package regService

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

var (
	fileContent = `[Unit]
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s
TimeoutStartSec=0
RestartSec=2
Restart=always

# Note that StartLimit* options were moved from "Service" to "Unit" in systemd 229.
# Both the old, and new location are accepted by systemd 229 and up, so using the old location
# to make them work for either version of systemd.
StartLimitBurst=3

# Note that StartLimitInterval was renamed to StartLimitIntervalSec in systemd 230.
# Both the old, and new name are accepted by systemd 230 and up, so using the old name to make
# this option work for either version of systemd.
StartLimitInterval=60s

KillMode=process
OOMScoreAdjust=-500

[Install]
WantedBy=multi-user.target`
)

func WriteToFile() {
	switch runtime.GOOS {
	case "darwin":
		if err := InstallAutoStart(); err != nil {
			println("macOS 登录自启注册失败:", err.Error())
			return
		}
		println("macOS 登录自启已注册")
	case "linux":
		writeToLinux()
	case "windows":
		writeToWindows()
	default:
		println("不支持的操作系统:", runtime.GOOS)
	}
}

func writeToLinux() {
	var err error
	execPath, err := filepath.Abs(os.Args[0])
	baseName := filepath.Base(os.Args[0])
	println("文件路径为：", execPath)
	if err != nil {
		println("注册失败")
		return
	}
	serviceName := baseName + ".service"
	targetFile := "/usr/lib/systemd/system/" + serviceName
	writeTxt := []byte(fmt.Sprintf(fileContent, execPath))
	err = os.WriteFile(targetFile, writeTxt, 777)
	if err != nil {
		println("无法产生文件", targetFile, err.Error())
		return
	}
	println("生成成功 服务为:", serviceName)
	runExec("关闭"+serviceName, "systemctl", "stop", serviceName)
	runExec("设置开机运行", "systemctl", "enable", serviceName)
	runExec("尝试启动", "systemctl", "start", serviceName)
}

func writeToWindows() {
	execPath, err := filepath.Abs(os.Args[0])
	if err != nil {
		println("获取可执行文件路径失败:", err.Error())
		return
	}

	baseName := filepath.Base(os.Args[0])
	println("文件路径为：", execPath)

	// 获取用户主目录
	homeDir, err := os.UserHomeDir()
	if err != nil {
		println("获取用户主目录失败:", err.Error())
		return
	}

	// 构建Windows启动文件夹路径
	startupDir := filepath.Join(homeDir, "AppData", "Roaming", "Microsoft", "Windows", "Start Menu", "Programs", "Startup")

	// 确保启动文件夹存在
	err = os.MkdirAll(startupDir, 0755)
	if err != nil {
		println("创建启动文件夹失败:", err.Error())
		return
	}

	// 目标文件路径（在启动文件夹中）
	targetFile := filepath.Join(startupDir, baseName)

	// 复制可执行文件到启动文件夹
	err = copyFile(execPath, targetFile)
	if err != nil {
		println("复制文件到启动文件夹失败:", err.Error())
		return
	}

	println("成功添加到Windows启动项:", targetFile)
	println("Windows开机自启动设置完成")
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

func runExec(remark, execS string, arg ...string) {
	command := exec.Command(execS, arg...)
	err := command.Run()
	r := err == nil
	println(remark, ":", r)
}
