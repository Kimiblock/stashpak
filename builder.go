package main

import (
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// Prepare a Git repository for dependency building, returns the build directory
func prepDepRepo(debug *log.Logger, warn *log.Logger, pkgname string, url string) (string) {
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
	cleanDir(path, debug, warn)
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

// Builds a package in a git repository, returns a slice of package files. This function does not resolve dependencies.
// Warning: prefix should be set!
// pkgname can be empty or base or actual name
func build(debug *log.Logger, warn *log.Logger, pkgname string, path string, prefix string, deps []pkginfo) []string {
	debug.Println("Building package", pkgname, "with dependency list:", deps)
	var elereq elevateRequest
	elereq.wd = path
	elereq.cmdline = []string{prefix, "--"}
	for _, dep := range deps {
		elereq.cmdline = append(elereq.cmdline, "-I", dep.pkgname)
	}
	elereq.cmdline = append(elereq.cmdline, "--", "PKGEXT=.pkg.tar")
	elereq.err = make(chan error, 1)
	elevate <- elereq
	err := <- elereq.err
	if err != nil {
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
