package main

import (
	"log"
	"os/exec"
	"syscall"
	"os"
	"errors"
	"path/filepath"
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
		delPkgs(path, debug, warn)
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
		delPkgs(path, debug, warn)
	}
}

func getRemoteGit(path string, gitInf gitInfo) error {
	err := os.RemoveAll(path)
	if os.IsNotExist(err) {} else if err != nil {
		return errors.New("Could not remove previous repository: " + err.Error())
	}
	cmdline := []string{
		"clone",
	}
	cmdline = append(cmdline, gitInf.uri, path, "--depth=1")
	if ! gitInf.defaultBranch {
		cmdline = append(
			cmdline,
			"-b",
			gitInf.branch,
		)
	}

	cmd := exec.Command("git", cmdline...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig:	syscall.SIGTERM,
	}
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	err = cmd.Run()
	if err != nil {
		return errors.New("Could not download repository: " + err.Error())
	}
	return nil
}

func updateRepo(debug *log.Logger, warn *log.Logger) {
	cancelFunc := obtainLock(debug, warn, "repo")
	defer cancelFunc()
	path := filepath.Join(
		xdgDir.cacheDir,
		"stashpak",
		"repo",
	)
	wd := filepath.Join(
		xdgDir.cacheDir,
		"stashpak",
	)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		err = os.MkdirAll(wd, 0700)
		if err != nil {
			warn.Fatalln("Could not create cache directory:", err)
		}
		cmdl := []string{
			"clone",
			repoUrl,
			"repo",
			"--depth=1",
		}
		cmd := exec.Command("git", cmdl...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Dir = wd
		err = cmd.Run()
		if err != nil {
			warn.Fatalln("Could not clone repository:", err)
		}
	} else if err != nil {
		warn.Fatalln("Could not stat repo:", err)
	}


	cmdline := []string{
		"pull",
	}

	cmd := exec.Command("git", cmdline...)
	cmd.Dir = path
	debug.Println("Updating local copy of repository...")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	err = cmd.Run()
	if err != nil {
		warn.Println("Could not update local copy of repository:", err)
	}
}
