package main

import (alpm "github.com/Jguer/go-alpm/v2")

// #include <alpm.h>
import "C"
import (
	"unsafe"
)

func dummyFunc() {
	alpm.Initialize("/", "/var/lib/pacman")
}

// true if ver1 is newer than ver2
func isNewer(v1 string, v2 string) bool {
	oldS := C.CString(v1)
	newS := C.CString(v2)
	res := C.alpm_pkg_vercmp(oldS, newS)
	C.free(unsafe.Pointer(oldS))
	C.free(unsafe.Pointer(newS))
	if int(res) > 0 {
		return true
	} else {
		return false
	}
}