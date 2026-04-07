package main

// #include <alpm.h>
import "C"
import (
	"unsafe"
)

func isNewer(old string, new string) bool {
	oldS := C.CString(old)
	newS := C.CString(new)
	res := C.alpm_pkg_vercmp(oldS, newS)
	C.free(unsafe.Pointer(oldS))
	C.free(unsafe.Pointer(newS))
	if int(res) < 0 {
		return true
	} else {
		return false
	}
}