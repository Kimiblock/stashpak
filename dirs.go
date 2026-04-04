package main

import (
	"log"
	"os"
	"sync"
	"strings"
	"path/filepath"
)

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