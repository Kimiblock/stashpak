package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// Obtain a file lock, caller should call cancelFunc
func obtainLock(operationName string) (func () (), error) {
	path := filepath.Join(xdgDir.stateHome, "StashPak", operationName, ".lock")
	err := os.MkdirAll(filepath.Dir(path), 0700)
	if err != nil {
		return nil, err
	}
	type result struct {
		err		error
	}
	resChan := make(chan result, 2)
	var file *os.File
	go func () {
		file, err = os.OpenFile(
			path,
			os.O_CREATE|os.O_RDWR,
			0700,
		)
		resChan <- result {
			err:	err,
		}
	} ()
	for sig := range resChan {
		if sig.err != nil {
			return nil, sig.err
		}
		break
	}
	err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
	if err != nil {
		return nil, err
	}
	return func() {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		if err != nil {
			panic("Could not release lock: " + err.Error())
		}
		file.Close()
	}, nil
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