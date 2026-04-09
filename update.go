package main

import (
	"bufio"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

func parsePkgsList(cmd *exec.Cmd) []installedPackage {
	var wg sync.WaitGroup
	var wgAppend sync.WaitGroup
	cmd.Stderr = os.Stderr
	outRaw, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	rd := bufio.NewScanner(outRaw)
	go func () {
		cmd.Run()
	} ()
	var pkgsChan = make(chan installedPackage, 512)
	var pkgsList []installedPackage
	wgAppend.Go(func() {
		for sig := range pkgsChan {
			pkgsList = append(pkgsList, sig)
		}
	})
	for rd.Scan() {
		line := rd.Text()
		wg.Go(func() {
		if len(strings.TrimSpace(line)) == 0 {
			return
		}
		sp := strings.Split(line, " ")
		if len(sp) != 2 {
			return
		}
		pkg := strings.TrimSpace(sp[0])
		ver := strings.TrimSpace(sp[1])
		pkgsChan <- installedPackage{
			name:		pkg,
			installedVer:	ver,
		}
		})
	}

	wg.Wait()
	close(pkgsChan)
	wgAppend.Wait()
	return pkgsList
}

func listPortablePkgs(logger, warn *log.Logger) []installedPackage {
	var pkgsChan = make(chan installedPackage, 512)
	arch, err := getArch()

	if err != nil {
		warn.Fatalln("Could not list package:", err)
	}

	repoDir := filepath.Join(xdgDir.cacheDir, "stashpak", "repo", arch)
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		warn.Fatalln("Could not read repository directory:", err)
	}
	var wg sync.WaitGroup
	for _, ent := range entries {
		wg.Go(func() {
			if ! ent.IsDir() {
				return
			}
			confPath := filepath.Join(
				repoDir,
				ent.Name(),
				"stashpak.toml",
			)
			confFile, err := os.OpenFile(
				confPath,
				os.O_RDONLY,
				0700,
			)
			if err != nil {
				if os.IsNotExist(err) {
					warn.Println(ent.Name(), "is not StashPak ready")
					return
				}
				warn.Println("Could not read", confFile, ":", err)
				return
			}
			defer confFile.Close()
			var conf pkgConf
			reader := bufio.NewReader(confFile)
			dec := toml.NewDecoder(reader)
			md, err := dec.Decode(&conf)
			if err != nil {
				warn.Println("Package", ent.Name(), "is broken:", err)
				return
			}
			if len(md.Undecoded()) > 0 {
				warn.Println("Package", ent.Name(), "is broken with unknown content:", md.Undecoded())
			}
			if conf.Metadata.Type == "repo" {
				return
			}
			srcPath := filepath.Join(
				repoDir,
				ent.Name(),
				".SRCINFO",
			)
			srcinfo, err := os.OpenFile(
				srcPath,
				os.O_RDONLY,
				0700,
			)
			if err != nil {
				warn.Println("Package", ent.Name(), "is broken with unreadable SRCINFO")
			}
			defer srcinfo.Close()
			scanner := bufio.NewScanner(srcinfo)
			var pkgver string
			var pkgrel string
			var epoch string
			type info struct {
				pkgver	bool
				epoch	bool
				pkgrel	bool
			}
			var inf info
			for scanner.Scan() {
				line := strings.TrimPrefix(scanner.Text(), "	")
				if strings.HasPrefix(strings.TrimSpace(line), "pkgver") && ! inf.pkgrel {
					sp := strings.Split(line, " ")
					if len(sp) != 3 {
						warn.Println("Could not decode SRCINFO: column mismatch")
						return
					}
					pkgver = sp[2]
					inf.pkgver = true
					continue
				}
				if strings.HasPrefix(strings.TrimSpace(line), "pkgrel") && ! inf.pkgrel {
					sp := strings.Split(line, " ")
					if len(sp) != 3 {
						warn.Println("Could not decode SRCINFO: column mismatch")
						return
					}
					pkgrel = sp[2]
					inf.pkgrel = true
					continue
				}
				if strings.HasPrefix(strings.TrimSpace(line), "epoch") && ! inf.epoch {
					sp := strings.Split(line, " ")
					if len(sp) != 3 {
						warn.Println("Could not decode SRCINFO: column mismatch")
						return
					}
					epoch = sp[2]
					inf.epoch = true
					continue
				}
			}
			var finalVer string
			if inf.epoch {
				finalVer = epoch + ":"
			}
			if inf.pkgver {
				finalVer = finalVer + pkgver
			} else {
				warn.Println("Could not decode SRCINFO: version unknown")
				return
			}
			if inf.pkgrel {
				finalVer = finalVer + "-" + pkgrel
			}
			pkgsChan <- installedPackage{
				name:		ent.Name(),
				repoVer:	finalVer,
			}
		})
	}
	var retChan = make(chan []installedPackage, 1)
	go func () {
		var ret []installedPackage
		for sig := range pkgsChan {
			ret = append(ret, sig)
		}
		retChan <- ret
	} ()
	wg.Wait()
	close(pkgsChan)

	return <- retChan
}

// Gets installed Portable packages list
func getPkgsList(logger *log.Logger, warn *log.Logger) []installedPackage {
	cmd := exec.Command("pacman", "-Q")
	logger.Println("Parsing package list")
	list := parsePkgsList(cmd)
	listPortable := listPortablePkgs(logger, warn)
	logger.Println("Got", len(list), "local packages")

	var retWg sync.WaitGroup
	retWg.Add(1)
	var retChan = make(chan installedPackage, 512)
	var ret []installedPackage
	go func () {
		defer retWg.Done()
		for sig := range retChan {
			ret = append(ret, sig)
		}
	} ()
	var wg sync.WaitGroup
	for _, val := range list {
		valUpper := val
		wg.Go(func() {
			var isRepo bool
			var repoDat installedPackage
			var localDat installedPackage = valUpper
			for _, val := range listPortable {
				if val.name == valUpper.name {
					isRepo = true
					repoDat = val
					break
				}
			}
			if isRepo {
				var hasUpd bool
				if isNewer(repoDat.repoVer, localDat.installedVer) {
					hasUpd = true
				}
				retChan <- installedPackage{
					name:		localDat.name,
					installedVer:	localDat.installedVer,
					repoVer:	repoDat.repoVer,
					hasUpdate:	hasUpd,
				}
			}
		})
	}
	wg.Wait()
	close(retChan)
	retWg.Wait()
	return ret
}