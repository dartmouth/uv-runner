//go:build windows

package main

import (
	"os/exec"
)

func (a *App) setupUnixProcessGroup(cmd *exec.Cmd) {
	// On Windows, we don't use process groups the same way
	// Process cleanup is handled differently in the cleanup() function
}
