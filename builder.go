package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Prepare a Git repository for dependency building, returns the build directory
func prepDepRepo(debug *log.Logger, warn *log.Logger, pkgname string, gitInf gitInfo) (string) {
	errChan := make(chan error, 16)
	go func () {
		for sig := range errChan {
			if sig != nil {
				warn.Fatalln("Could not prepare git repository:", sig)
			}
		}
	} ()
	var path string = filepath.Join(xdgDir.cacheDir, "stashpak/git", pkgname)
	debug.Println("Preparing a build directory...")
	cmdline := []string{
		"-C", path,
		"remote",
		"get-url",
		"origin",
	}
	cmd := exec.Command("git", cmdline...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig:		syscall.SIGTERM,
	}
	out, err := cmd.Output()
	if err != nil {
		debug.Println("Could not get origin URL of repository:", err)
		err = getRemoteGit(path, gitInf)
		if err != nil {
			errChan <- err
			warn.Println(err)
		}
	} else if strings.TrimSpace(string(out)) != gitInf.uri {
		warn.Println(
			"Repository origin mismatch:",
			string(out),
			"!=",
			gitInf.uri,
		)
		err := getRemoteGit(path, gitInf)
		if err != nil {
			warn.Println(err)
			errChan <- err
		}
	}
	cleanDir(path, debug, warn)
	cmdline = []string{
		"-C", path,
		"pull",
	}
	cmd = exec.Command("git", cmdline...)
	cmd.Dir = path
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig:		syscall.SIGTERM,
	}
	err = cmd.Run()
	if err != nil {
		warn.Println("Could not update repository:", err)
		errChan <- err
	}

	if ! gitInf.defaultBranch {
		cmd := exec.Command(
			"git",
			"-C", path,
			"fetch", "origin", gitInf.branch)
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			warn.Fatalln(
				"Could not fetch branch",
				strconv.Quote(gitInf.branch),
				":", err,
				)
		}
		cmd = exec.Command("git", "-C", path, "switch", gitInf.branch)
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			warn.Fatalln(
				"Could not switch to branch",
				strconv.Quote(gitInf.branch),
				":", err,
				)
		}
	}
	close(errChan)
	return path
}

// Builds a package in a git repository, returns a slice of package files. This function does not resolve dependencies.
// Warning: prefix should be set!
// pkgname can be empty or base or actual name
func build(debug *log.Logger, warn *log.Logger, pkgname string, path string, prefix string, deps []pkginfo) []string {
	var cancelFunc func ()
	var lockDone = make(chan bool)
	var lockTimeout = make(chan bool)

	go func () {
		time.Sleep(90 * time.Second)
		lockTimeout <- true
	} ()

	go func() {
		defer func () {lockDone <- true} ()
		var err error
		cancelFunc, err = obtainLock(
			"build" + pkgname,
		)
		if err != nil {
			warn.Fatalln("Could not lock build path:", err)
		}
	} ()

	var locked bool

	select {
		case <- lockDone:
			locked = true
		case <- lockTimeout:
			warn.Println("Waited 90 seconds for build directory to lock")
	}

	if ! locked {
		<- lockDone
	}
	defer cancelFunc()

	debug.Println("Building package", pkgname, "with dependency list:", deps)
	var elereq elevateRequest
	elereq.wd = path
	elereq.cmdline = []string{prefix, "--"}
	for _, dep := range deps {
		elereq.cmdline = append(elereq.cmdline, "-I", dep.pkgname)
	}
	elereq.cmdline = append(elereq.cmdline, "--", "PKGEXT=.pkg.tar", "--skippgpcheck")
	elereq.err = make(chan error, 1)
	elereq.wantPipe = true
	elereq.pipeChan = make(chan cmdPipe)

	elevate <- elereq
	pipes := <- elereq.pipeChan
	err := <- elereq.err
	if err != nil {
		warn.Println("An Error occured while building package:", pkgname)
		scannerOut := bufio.NewScanner(pipes.stdoutPipe)
		for scannerOut.Scan() {
			fmt.Println("[stdout]:", scannerOut.Text())
		}

		scanner := bufio.NewScanner(pipes.stderrPipe)
		for scanner.Scan() {
			fmt.Println("[stderr]:", scanner.Text())
		}
		warn.Fatalln("Could not build package", pkgname, ":", err)
	}
	ent, err := os.ReadDir(path)
	var listChan = make(chan string, 10)
	var list []string
	go func () {
		for pkg := range listChan {
			list = append(list, pkg)
		}
	} ()
	var wg sync.WaitGroup
	for _, info := range ent {
		wg.Go(func() {
			if strings.Contains(info.Name(), ".pkg") && ! strings.HasSuffix(info.Name(), ".log") && info.IsDir() == false {
				listChan <- filepath.Join(path, info.Name())
			}
		})
	}
	wg.Wait()
	close(listChan)
	debug.Println("Built package", pkgname)
	return list
}