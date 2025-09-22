//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

func (a *App) setupUnixProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// Create new process group
	cmd.SysProcAttr.Setpgid = true
}
