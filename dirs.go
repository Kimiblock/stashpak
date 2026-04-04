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
				filepath.Join(file),
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

// Lists package files in a directory
func listPkgs(path string, debug *log.Logger, warn *log.Logger) []string {
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
	return list
}