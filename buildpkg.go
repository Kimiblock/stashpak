package main

import (
	"log"
	"path/filepath"
	"sync"
)

// Resolves dependencies and build packages
func resolveDeps(dep DependsSection, debug *log.Logger, warn *log.Logger) ([]pkginfo) {
	debug.Println("Obtaining dependency", dep.Pkgname)
	var wg sync.WaitGroup
	var deps []pkginfo
	var depsChan = make(chan []pkginfo, 4)
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
	switch dep.SourceType {
		case "git":
			gitInf := gitInfo{
				uri: dep.Source,
				branch: dep.Branch,
			}
			if len(dep.Branch) > 0 {
				gitInf.defaultBranch = false
			} else {
				gitInf.defaultBranch = true
			}
			pth := prepDepRepo(debug, warn, dep.Pkgname, gitInf)
			var pfx string
			if len(dep.BuildPrefix) == 0 {
				pfx = "extra-x86_64-build"
			} else {
				pfx = dep.BuildPrefix
			}
			depsList := build(
				debug,
				warn,
				dep.Pkgname,
				pth,
				pfx,
				deps,
			)
			if dep.Install {
				for _, val := range depsList {
					depsChan <- []pkginfo{
						{
							install: true,
							pkgname: val,
						},
					}
				}
			} else {
				for _, val := range depsList {
					depsChan <- []pkginfo{
						{
							install: false,
							pkgname: val,
						},
					}
				}
			}
		case "repo":
			pkgs := getPkg(debug, warn, dep.Source + "/" + dep.Pkgname)
			if dep.Install {
				for _, val := range pkgs {
					depsChan <- []pkginfo{
						{
							install: true,
							pkgname: val,
						},
					}
				}
			} else {
				for _, val := range pkgs {
					depsChan <- []pkginfo{
						{
							install: false,
							pkgname: val,
						},
					}
				}
			}
		default:
			warn.Fatalln("Invalid dependency type for", dep.Pkgname, ":", dep.SourceType)
	}
	close(depsChan)
	depsLock.Lock()
	depsLock.Unlock()
	debug.Println("Resolved dependency list for", dep.Pkgname, ":", deps)
	return deps
}

// The new-style builder function for building a package
func buildPackage(baseDir string, debug *log.Logger, warn *log.Logger) []pkginfo {
	var wg sync.WaitGroup
	var ret []pkginfo
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
		pkgs := getPkg(debug, warn, pkg.Metadata.Repo)

		for _, val := range pkgs {
			ret = append(ret, pkginfo{
				pkgname: val,
				install: true,
			})
		}
		return ret
	}

	depsChan := make(chan []pkginfo, 16)
	var deps []pkginfo
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
	//cleanDir(baseDir, debug, warn)
	var pfx string
	if len(pkg.Metadata.BuildPrefix) == 0 {
		pfx = "extra-x86_64-build"
	} else {
		pfx = pkg.Metadata.BuildPrefix
	}

	prods := build(
		debug,
		warn,
		"base",
		baseDir,
		pfx,
		deps,
		)

	for _, val := range prods {
		deps = append(deps, pkginfo{
			pkgname: val,
			install: true,
		})
	}

	return deps
}