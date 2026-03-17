package main

import (
	"bufio"
	"context"
	"errors"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	alpm "github.com/Jguer/go-alpm/v2"
)

var (
	conf		envConf
	xdgDir		xdg
	elevate		= make(chan elevateRequest, 2)
)

const (
	repoUrl		string = "https://github.com/Kraftland/portable-arch.git"
)

// Client must initialize success / error channel!
type elevateRequest struct {
	cmdline		[]string
	timeout		time.Duration
	err		chan error
}

type xdg struct {
	runtimeDir		string
	confDir			string
	cacheDir		string
	dataDir			string
	home			string
}

type pkgConf struct {
	// Arrays of tables containing dependencies not from core/extra
	Depends		[]DependsSection
	Metadata	confMeta
}

type confMeta struct {
	// The build prefix. Defaults to "extra-x86_64-build".
	BuildPrefix	string
	// GitHub user name of Maintainer
	Maintainer	string
}

type DependsSection struct {
	Pkgname		string
	// Source can be either a string of git URL (type git), or repo name (type repo) to download from locally defined repositories.
	SourceType	string
	Source		string
	// The build prefix for type git. Defaults to "extra-x86_64-build".
	BuildPrefix	string
}

type envConf struct {
	elevateProgram		string
}

func decodeConf (path string, warn *log.Logger) (pkgConf, error) {
	var res pkgConf
	res.Metadata.BuildPrefix = "extra-x86_64-build"
	file, err := os.Open(path)
	if err != nil {
		warn.Fatalln("Could not open package metadata:", err)
		return res, err
	}
	reader := bufio.NewReader(file)
	decoder := toml.NewDecoder(reader)
	meta, err := decoder.Decode(&res)
	if err != nil {
		warn.Fatalln("Could not decode package metadata:", err)
		return res, err
	}
	if len(meta.Undecoded()) > 0 {
		warn.Println("Undecoded content:", meta.Undecoded())
	}
	for idx, struc := range res.Depends {
		if len(struc.BuildPrefix) == 0 {
			res.Depends[idx].BuildPrefix = "extra-x86_64-build"
		}
	}
	return res, nil
}

func validateConf (path string, warn *log.Logger) []error {
	errChan := make(chan error, 32)
	con, err := decodeConf(path, warn)
	if err != nil {
		return []error{err}
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		_, err = exec.LookPath(con.Metadata.BuildPrefix)
		if err != nil {
			errChan <- errors.New("Build prefix for main package invalid: " + err.Error())
		}
	})
	wg.Go(func() {
		if len(con.Metadata.Maintainer) == 0 {
			errChan <-  errors.New("Maintainer not set")
		}
	})


	for _, stru := range con.Depends {
		wg.Go(func() {
			_, err = exec.LookPath(stru.BuildPrefix)
			if err != nil {
				errChan <- errors.New("Build prefix for " + stru.Pkgname + " invalid: " + err.Error())
			}
			if len(stru.Pkgname) == 0 {
				errChan <- errors.New("Invalid package name")
			}
			switch stru.SourceType {
				case "git":
					args := []string{"ls-remote", stru.Source}
					cmd := exec.Command("git", args...)
					cmd.Stderr = os.Stderr
					err := cmd.Run()
					if err != nil {
						errChan <- errors.New("Could not get status of " + err.Error())
					}
				case "repo":
					args := []string{"-Si", stru.Source + "/" + stru.Pkgname}
					cmd := exec.Command("pacman", args...)
					cmd.Stderr = os.Stderr
					err := cmd.Run()
					if err != nil {
						errChan <- errors.New("Package" + stru.Pkgname + " could not be found")
					}
			}
		})

	}


	go func () {
		wg.Wait()
		close(errChan)
	} ()
	var ret []error
	for sig := range errChan {
		ret = append(ret, sig)
	}
	return ret
}

func elevator(debug *log.Logger, warn *log.Logger) {
	var hasLoop bool
	for sig := range elevate {
		if hasLoop == false {
			debug.Println("Starting elevate loop")
			hasLoop = true
			time.Sleep(2 * time.Minute)
			for {
				ctx := context.TODO()
				ctxNew, canc := context.WithTimeout(ctx, 5 * time.Second)
				cmd := exec.CommandContext(ctxNew, conf.elevateProgram, "true")
				cmd.Stderr = os.Stderr
				err := cmd.Run()
				canc()
				if err != nil {
					warn.Println("Could not loop elevate status:", err)
					break
				}
			}
		}
		go func () {
			signal := sig
			if signal.timeout == 0 {
				cmd := exec.Command(signal.cmdline[0], signal.cmdline[1:]...)
				cmd.Stderr = os.Stderr
				err := cmd.Run()
				if err != nil {
					warn.Println("Elevated command has failed:", err)
					signal.err <- err
				} else {
					signal.err <- nil
				}
			} else {
				ctx := context.TODO()
				ctxTimeout, cancelFunc := context.WithTimeout(ctx, signal.timeout)
				cmd := exec.CommandContext(ctxTimeout, signal.cmdline[0], signal.cmdline[1:]...)
				err := cmd.Run()
				cancelFunc()
				if err != nil {
					warn.Println("Elevated command has failed:", err)
					signal.err <- err
				} else {
					signal.err <- nil
				}
			}
		} ()
	}
}

func getRemoteGit(path string, url string) error {
	err := os.RemoveAll(path)
	if os.IsNotExist(err) {} else {
		return errors.New("Could not remove previous repository: " + err.Error())
	}
	cmdline := []string{
		"clone",
		url,
		path,
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

// Builds a package from git repository using chroot
func buildPkg(debug *log.Logger, warn *log.Logger, pkgname string, url string, prefix string) {
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
			warn.Fatalln(err)
		}
	} else if string(out) != url {
		warn.Println("Repository mismatch, downloading from source")
		err := getRemoteGit(buildPath, url)
		if err != nil {
			warn.Fatalln(err)
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
		warn.Fatalln("Could not update repository:", err)
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
			warn.Fatalln("Could not remove previous build directory:", err)
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
		warn.Fatalln("Could not create working copy:", err)
	}

	var elereq elevateRequest
	elereq.cmdline = []string{prefix}
	elereq.err = make(chan error, 1)
	elevate <- elereq
	err = <- elereq.err
	if err != nil {
		warn.Fatalln("Could not build package", pkgname, ":", err)
	}

}

func updateRepo(debug *log.Logger, warn *log.Logger) {
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
		warn.Fatalln("Could not update local copy of repository:", err)
	}

}

func pickBuildDir(warn *log.Logger, pkgname string) string {
	stat, err := os.Stat(xdgDir.cacheDir)
	if err != nil {
		warn.Fatalln("Could not stat XDG Cache Directory:", err)
	}
	if stat.IsDir() == false {
		warn.Fatalln("XDG Cache Directory invalid: not a directory")
	}
	path := filepath.Join(xdgDir.cacheDir, "stashpak", "git", pkgname)
	err = os.MkdirAll(path, 0700)
	if err != nil {
		warn.Fatalln("Could not create build path:", err)
	}
	return path
}

// Returns the absolute location of a package file
func getPkg(debug *log.Logger, warn *log.Logger, pkgname string) string {
	debug.Println("Obtaining package file for", pkgname)
	cmdline := []string{"pacman", "-Spw", pkgname}
	ctx := context.TODO()
	ctxNew, cancelFunc := context.WithTimeout(ctx, 5 * time.Second)
	cmd := exec.CommandContext(ctxNew, cmdline[0], cmdline[1:]...)
	out, err := cmd.Output()
	cancelFunc()
	if err != nil {
		warn.Fatalln("Command", cmdline, "has failed:", err)
	}
	res, present := strings.CutPrefix(string(out), "https://")
	if present {
		var req elevateRequest
		req.err = make(chan error, 1)
		req.cmdline = []string{
			"pacman",
			"-Sw",
			pkgname,
		}
		elevate <- req
		err := <- req.err
		if err != nil {
			warn.Fatalln("Could not download package:", err)
		}
	}
	res, present = strings.CutPrefix(string(out), "file://")
	if present {
		return res
	} else {
		warn.Fatalln("Could not download package:", "pacman returned unknown URI:", res)
		return res
	}
}

func lookUpXDG(debug *log.Logger, warn *log.Logger) {
	xdgDir.runtimeDir = os.Getenv("XDG_RUNTIME_DIR")
	if len(xdgDir.runtimeDir) == 0 {
		warn.Fatalln("XDG_RUNTIME_DIR not set")
	} else {
		runtimeDirInfo, errRuntimeDir := os.Stat(xdgDir.runtimeDir)
		if errRuntimeDir != nil {
			warn.Fatalln("Could not determine the status of XDG Runtime Directory", errRuntimeDir)
		}
		if runtimeDirInfo.IsDir() == false {
			warn.Fatalln("XDG_RUNTIME_DIR is not a directory")
		}
	}

	var cacheErr error
	var homeErr error
	var confErr error
	xdgDir.home, homeErr = os.UserHomeDir()
	if homeErr != nil {
		warn.Fatalln("Could not determine user home:", homeErr)
	}

	xdgDir.cacheDir, cacheErr = os.UserCacheDir()
	if cacheErr != nil {
		warn.Fatalln("Could not find XDG cache directory:", cacheErr)
	}

	xdgDir.confDir, confErr = os.UserConfigDir()
	if confErr != nil {
		warn.Fatalln("Could not find XDG config home:", confErr)
	}

	datahome := os.Getenv("XDG_DATA_HOME")
	if len(datahome) > 0 {
		xdgDir.dataDir = datahome
	} else {
		xdgDir.dataDir = xdgDir.home + "/.local/share"
		debug.Println("Using default data home: " + xdgDir.dataDir)
	}
}

func processOpts(logger *log.Logger) {
	elevate := os.Getenv("stashPakElevateProgram")
	if len(elevate) > 0 {
		if path, err := exec.LookPath(elevate); err == nil {
			conf.elevateProgram = path
		} else {
			logger.Println("Could not resolve elevate binary path:", err)
		}

	}
}

func cmdlineDispatcher(logger *log.Logger, warn *log.Logger) {
	cmdSlice := os.Args[1:]
	logger.Println()


	action := cmdSlice[0]
	switch action {
		case "validate":
			for _, file := range cmdSlice[1:] {
				logger.Println("Checking configuration:", file)
				errs := validateConf(file, warn)
				if len(errs) > 0 {
					warn.Println("Configuration", file, "failed to pass validation:", errs)
				}
			}

	}
}

func main () {
	debug := log.New(os.Stdout, "[StashPak]: ", 0)
	warn := log.New(os.Stderr, "[Warning] [StashPak]: ", 0)
	lookUpXDG(debug, warn)
	processOpts(debug)
	go elevator(debug, warn)

	handler, err := alpm.Initialize("/", "/var/lib/pacman")
	if err != nil {
		panic("Could not initialize alpm: " + err.Error())
	}
	defer handler.Release()
	db, err := handler.LocalDB()
	if err != nil {
		panic("Could not initialize alpm: " + err.Error())
	}
	debug.Println("Initialized ALPM handler for database:", db.Name())
	updateRepo(debug, warn)
	cmdlineDispatcher(debug, warn)
}