package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"syscall"
)

// Obtain a file lock, caller should call cancelFunc
func obtainLock(debug *log.Logger, warn *log.Logger) (cancelFunc func () ()) {
	path := filepath.Join(xdgDir.stateHome, "StashPak", "op.lock")
	err := os.MkdirAll(filepath.Dir(path), 0700)
	if err != nil {
		warn.Fatalln("Could not obtain lock:", err)
	}
	type result struct {
		timeout		bool
	}
	resChan := make(chan result, 2)
	go func () {
		time.Sleep(1 * time.Minute)
		resChan <- result{
			timeout: true,
		}
	} ()
	var file *os.File
	go func () {
		file, err = os.OpenFile(
			path,
			os.O_CREATE|os.O_RDWR,
			0700,
		)
		if err != nil {
			warn.Fatalln("Could not obtain lock:", err)
		}
		resChan <- result{
			timeout: false,
		}
	} ()
	for sig := range resChan {
		if sig.timeout {
			warn.Println("Waiting for file lock to release:", path)
		} else {
			break
		}
	}
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
	if err != nil {
		warn.Fatalln("Could not obtain lock:", err)
	}
	return func() {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		if err != nil {
			warn.Fatalln("Could not release lock:", err)
		}
		file.Close()
	}
}

// Deletes package files in a directory
func delPkgs(path string, debug *log.Logger, warn *log.Logger) {
	var wg sync.WaitGroup
	files := listPkgs(path, debug, warn)
	for _, file := range files {
		wg.Go(func() {
			err := os.RemoveAll(
				filepath.Join(file.pkgname),
			)
			if err != nil {
				warn.Fatalln("Could not remove a package file:", err)
			} else {
				debug.Println("Removed", file)
			}
		})
	}
	wg.Wait()
}

// Lists package files in a directory, info.install will be true
func listPkgs(path string, debug *log.Logger, warn *log.Logger) []pkginfo {
	ent, err := os.ReadDir(path)
	if err != nil {
		warn.Fatalln("Could not read directory:", err)
	}
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
	var inf []pkginfo
	for _, name := range list {
		inf = append(inf, pkginfo{
			pkgname:	name,
			install:	true,
		})
	}
	return inf
}

// Installs packages from a slice
func instSlice(pkgslice []pkginfo, debug *log.Logger, warn *log.Logger) {
	var elereq elevateRequest
	elereq.cmdline = []string{"pacman", "--noconfirm", "-U"}
	var pkgs []string
	for _, pkg := range pkgslice {
		if pkg.install {
			debug.Println("Adding package", pkg.pkgname, "to list")
			pkgs = append(pkgs, pkg.pkgname)
		}
	}
	if len(pkgs) == 0 {
		return
	}
	elereq.cmdline = append(elereq.cmdline, pkgs...)
	elereq.err = make(chan error, 1)
	elevate <- elereq
	err := <- elereq.err
	if err != nil {
		warn.Fatalln("Could not install packages:", err)
	}
}