package main

import (
	"github.com/v2fly/v2ray-core/v5/main/commands"
	_ "github.com/v2fly/v2ray-core/v5/main/commands/all"
	"github.com/v2fly/v2ray-core/v5/main/commands/base"
)

// go build -o build/v2ctl.exe  github.com/v2fly/v2ray-core/v5/main/z/ctl
func main() {
	base.RootCommand.Long = "A unified platform for anti-censorship."
	base.RegisterCommand(commands.CmdRun)
	base.RegisterCommand(commands.CmdVersion)
	base.RegisterCommand(commands.CmdTest)
	base.SortCommands()
	base.Execute()
}
