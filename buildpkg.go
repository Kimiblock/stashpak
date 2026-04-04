package main

import (
	"log"
	"path/filepath"
	"sync"
)

// Resolves dependencies and build packages
func resolveDeps(dep DependsSection, debug *log.Logger, warn *log.Logger) ([]string) {
	debug.Println("Obtaining dependency", dep.Pkgname)
	var wg sync.WaitGroup
	var depsChan = make(chan []string, 4)
	var deps []string
	var depsLock sync.RWMutex
	depsLock.Lock()
	go func () {
		for dep := range depsChan {
			deps = append(deps, dep...)
		}
		depsLock.Unlock()
	} ()
	for _, depInfo := range dep.Depends {
		wg.Go(func() {
			depsChan <- resolveDeps(depInfo, debug, warn)
		})
	}

	wg.Wait()
	close(depsChan)
	debug.Println("Resolved dependency list for", dep.Pkgname, ":", deps)
	switch dep.SourceType {
		case "git":
			pth := prepDepRepo(debug, warn, dep.Pkgname, dep.Source)
			var pfx string
			if len(dep.BuildPrefix) == 0 {
				pfx = "extra-x86_64-build"
			} else {
				pfx = dep.BuildPrefix
			}

			depsChan <- build(
				debug,
				warn,
				dep.Pkgname,
				pth,
				pfx,
				deps,
			)
		case "repo":
			depsChan <- getPkg(debug, warn, dep.Source + "/" + dep.Pkgname)
		default:
			warn.Fatalln("Invalid dependency type for", dep.Pkgname, ":", dep.SourceType)
	}
	close(depsChan)
	depsLock.Lock()
	depsLock.Unlock()
	return deps
}

// The new-style builder function for building a package
func buildPackage(baseDir string, debug *log.Logger, warn *log.Logger) []string {
	var wg sync.WaitGroup
	wg.Go(func() {
		err := validateConf(filepath.Join(baseDir, "stashpak.toml"), warn)
		if err != nil {
			warn.Fatalln("Validation of configuration failed:", err)
		}
	})
	pkg, err := decodeConf(filepath.Join(baseDir, "stashpak.toml"), warn)
	if err != nil {
		warn.Fatalln("Could not decode configuration:", err)
	}
	if pkg.Metadata.Type == "repo" {
		return getPkg(debug, warn, pkg.Metadata.Repo)
	}
	depsChan := make(chan []string, 16)
	var deps []string
	var depsLock sync.Mutex
	depsLock.Lock()
	go func () {
		defer depsLock.Unlock()
		for dep := range depsChan {
			deps = append(deps, dep...)
		}
	} ()
	for _, depInfo := range pkg.Depends {
		wg.Go(func() {
			depsChan <- resolveDeps(depInfo, debug, warn)
		})
	}
	wg.Wait()
	close(depsChan)
	depsLock.Lock()
	depsLock.Unlock()
	cleanDir(baseDir, debug, warn)
	var pfx string
	if len(pkg.Metadata.BuildPrefix) == 0 {
		pfx = "extra-x86_64-build"
	} else {
		pfx = pkg.Metadata.BuildPrefix
	}
	depsChan <- build(
		debug,
		warn,
		"base",
		baseDir,
		pfx,
		deps,
	)
	close(depsChan)
	depsLock.Lock()
	depsLock.Unlock()
	return deps
}