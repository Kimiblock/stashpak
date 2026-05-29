package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"text/tabwriter"

	"github.com/BurntSushi/toml"
	alpm "github.com/Jguer/go-alpm/v2"
)

func decodeConf (path string, warn *log.Logger) (pkgConf, error) {
	var res pkgConf
	res.Metadata.Type = "build"
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

// Returns the absolute location of a package file
func getPkg(debug *log.Logger, warn *log.Logger, pkgname string) []string {
	var ret []string

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
	split := strings.SplitSeq(string(out), "\n")
	var redownload bool
	for sp := range split {
		if strings.HasPrefix(sp, "http") {
			redownload = true
			break
		}
	}
	if redownload {
		var req elevateRequest
		req.err = make(chan error, 1)
		req.cmdline = []string{
			"pacman",
			"-Sw",
			"--noconfirm",
			pkgname,
		}
		pacmanLock.Lock()
		elevate <- req
		err := <- req.err
		if err != nil {
			warn.Fatalln("Could not download package:", err)
		}
		pacmanLock.Unlock()
	}
	ctx = context.TODO()
	ctxNew, cancelFunc = context.WithTimeout(ctx, 5 * time.Second)
	cmd = exec.CommandContext(ctxNew, cmdline[0], cmdline[1:]...)
	out, err = cmd.Output()
	cancelFunc()
	if err != nil {
		warn.Fatalln("Command", cmdline, "has failed:", err)
	}
	split = strings.SplitSeq(string(out), "\n")
	for sp := range split {
		if strings.HasPrefix(sp, "file://") {
			ret = append(ret, strings.TrimPrefix(sp, "file://"))
		} else if len(strings.TrimSpace(sp)) == 0 {
			continue
		} else if strings.TrimSpace(sp) == "\n" {
			continue
		} else {
			warn.Fatalln("Could not get location for package: unrecognized string:", sp)
		}
	}
	return ret
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

	statehome := os.Getenv("XDG_STATE_HOME")
	if len(statehome) > 0 {
		xdgDir.stateHome = statehome
	} else {
		xdgDir.stateHome = xdgDir.home + "/.local/state"
		debug.Println("Using default state home: " + xdgDir.stateHome)
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

	} else {
		conf.elevateProgram = "run0"
	}
}

func getArch() (string, error) {
	arch := runtime.GOARCH
	// From `go tool dist list`
	switch arch {
		case "amd64":
			return "x86_64", nil
		default:
			return "", errors.New("Unsupported architecture: " + arch)
	}
}

// Attempts to build and install one or more Portable Arch packages
func buildRepoPkgs(debug *log.Logger, warn *log.Logger, pkgs []string) error {
	var wg sync.WaitGroup
	arch, err := getArch()

	if err != nil {
		warn.Fatalln("Could not build package:", err)
	}
	baseDir := filepath.Join(xdgDir.cacheDir, "stashpak", "repo", arch)
	var pkgsChan = make(chan []pkginfo, 5)
	var pkgFiles []pkginfo
	var pkgsLock sync.Mutex
	pkgsLock.Lock()
	go func () {
		defer pkgsLock.Unlock()
		for pkg := range pkgsChan {
			pkgFiles = append(pkgFiles, pkg...)
		}
	} ()
	for _, pkg := range pkgs {
		pkgsChan <- buildPackage(
			filepath.Join(baseDir, pkg),
			debug,
			warn,
		)
	}

	wg.Wait()
	close(pkgsChan)
	pkgsLock.Lock()
	instSlice(pkgFiles, debug, warn)
	return nil

}
func cmdlineDispatcher(logger *log.Logger, warn *log.Logger) {
	if len(os.Args) < 2 {
		updateRepo(logger, warn)
		warn.Fatalln("StashPak requires an action")
	}
	cmdSlice := os.Args[1:]
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
		case "install-local", "build", "build-local":
			wd, err := os.Getwd()
			if err != nil {
				warn.Fatalln("Could not get working directory:", err)
			}
			deps := buildPackage(wd, logger, warn)
			instSlice(deps, logger, warn)
		case "get", "install":
			updateRepo(logger, warn)
			if len(cmdSlice) < 2 {
				warn.Fatalln("Action get requires one or more arguments")
			}
			err := buildRepoPkgs(logger, warn, cmdSlice[1:])
			if err != nil {
				warn.Fatalln(err)
			}
		case "update", "upgrade":
			updateRepo(logger, warn)
			timeNow := time.Now()
			pkgs := getPkgsList(logger, warn)
			var updPkgs []string
			for _, pkg := range pkgs {
				if pkg.hasUpdate {
					updPkgs = append(updPkgs, pkg.name)
				}
			}
			pkgsNum := len(updPkgs)
			if pkgsNum == 0 {
				logger.Println("Up to date")
				return
			}
			err := buildRepoPkgs(logger, warn, updPkgs)
			if err != nil {
				warn.Fatalln(err)
			}
			var trailingS string
			switch pkgsNum {
				case 0, 1:
					trailingS = ""
				default:
					trailingS = "s"
			}
			logger.Println("Updated", pkgsNum, "package" + trailingS, "in", time.Since(timeNow))
		case "list":
			updateRepo(logger, warn)
			pkgs := getPkgsList(logger, warn)
			w := tabwriter.NewWriter(os.Stdout, 20, 8, 8, '	', tabwriter.TabIndent)
			fmt.Fprintln(w, "Package Name\tVersion Installed\tVersion in Store\tHas Updates")
			for _, pkg := range pkgs {
				var hasUpd string
				switch pkg.hasUpdate {
					case true:
						hasUpd = "Yes"
					case false:
						hasUpd = "No"
				}
				fmt.Fprintln(w, pkg.name + "\t" + pkg.installedVer + "\t" + pkg.repoVer + "\t" + hasUpd)
			}
			w.Flush()
		default:
			warn.Fatalln("Could not execute action", action + ":", "unknown")

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
	cmdlineDispatcher(debug, warn)
}