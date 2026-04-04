package main

import (
	"log"
	"os/exec"
	"syscall"
)

// Reset + Clean
func cleanDir(path string, debug *log.Logger, warn *log.Logger) {
	cmdline := []string{
		"reset",
	}
	cmd := exec.Command("git", cmdline...)
	cmd.Dir = path
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig:		syscall.SIGTERM,
	}
	err := cmd.Run()
	if err != nil {
		warn.Println("Could not reset repository:", err)
	}
	cmdline = []string{
		"clean",
		"-fdx",
	}
	cmd = exec.Command("git", cmdline...)
	cmd.Dir = path
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig:		syscall.SIGTERM,
	}
	err = cmd.Run()
	if err != nil {
		warn.Println("Could not clean repository:", err)
	}
}