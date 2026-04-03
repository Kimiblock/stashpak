package main

import (
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Prepare a Git repository for dependency building, returns the build directory
func prepDepRepo(debug *log.Logger, warn *log.Logger, pkgname string, url string, prefix string) (string) {
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
		"remote",
		"get-url",
		"origin",
	}
	cmd := exec.Command("git", cmdline...)
	cmd.Dir = path
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig:		syscall.SIGTERM,
	}
	out, err := cmd.Output()
	if err != nil {
		debug.Println("Could not get origin URL of repository:", err)
		err = getRemoteGit(path, url)
		if err != nil {
			errChan <- err
			warn.Println(err)
		}
	} else if strings.TrimSpace(string(out)) != url {
		warn.Println("Repository origin mismatch:", string(out), "!=", url)
		err := getRemoteGit(path, url)
		if err != nil {
			warn.Println(err)
			errChan <- err
		}
	}
	cmdline = []string{
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
	close(errChan)
	return path


}

// Builds a package from git repository using chroot, returns the path to build directory and optionally a slice of errors
func buildPkg(debug *log.Logger, warn *log.Logger, pkgname string, url string, prefix string) (string, []error) {
	errChan := make(chan error, 16)
	cmdline := []string{
		"remote",
		"get-url",
		"origin",
	}
	buildPath := pickBuildDir(warn, pkgname)
	cmd := exec.Command("git", cmdline...)
	cmd.Dir = buildPath
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig:		syscall.SIGTERM,
	}
	out, err := cmd.Output()
	if err != nil {
		debug.Println("Could not get origin URL of repository:", err)
		err = getRemoteGit(buildPath, url)
		if err != nil {
			errChan <- err
			warn.Println(err)
		}
	} else if strings.TrimSpace(string(out)) != url {
		warn.Println("Repository mismatch, downloading from source")
		err := getRemoteGit(buildPath, url)
		if err != nil {
			warn.Println(err)
			errChan <- err
		}
	}


	cmdline = []string{
		"pull",
	}
	cmd = exec.Command("git", cmdline...)
	cmd.Dir = buildPath
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig:		syscall.SIGTERM,
	}
	err = cmd.Run()
	if err != nil {
		warn.Println("Could not update repository:", err)
		errChan <- err
	}

	debug.Println("Finished repository download")

	pathPfx := filepath.Join(
		xdgDir.cacheDir,
		"stashpak",
		"build",
	)


	buildDir := filepath.Join(pathPfx, strconv.Itoa(rand.Int()))
	_, err = os.Stat(buildDir)
	if os.IsNotExist(err) == false && err != nil {
		err := os.RemoveAll(buildDir)
		if err != nil {
			warn.Println("Could not remove previous build directory:", err)
			errChan <- err
		}
	}
	debug.Println("Creating a working copy of repository...")
	cloneCmd := []string{
		"clone",
		buildPath,
		buildDir,
	}

	cmd = exec.Command("git", cloneCmd...)
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		warn.Println("Could not create working copy:", err)
		errChan <- err
	}

	var elereq elevateRequest
	elereq.wd = buildDir
	elereq.cmdline = []string{prefix, "--", "--", "PKGEXT=.pkg.tar"}
	elereq.err = make(chan error, 1)
	elevate <- elereq
	err = <- elereq.err
	if err != nil {
		warn.Println("Could not build package", pkgname, ":", err)
		errChan <- err
	}

	go func () {
		close(errChan)
	} ()

	var ret []error

	for errSig := range errChan {
		ret = append(ret, errSig)
	}
	return buildDir, ret

}
