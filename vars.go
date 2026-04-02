package main

import (
	"sync"
)

var (
	pacmanLock	sync.Mutex
)