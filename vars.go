package main

import (
	"io"
	"sync"
	"syscall"
	"time"
)

var (
	pacmanLock	sync.Mutex
	conf		envConf
	xdgDir		xdg
	elevate		= make(chan elevateRequest, 2)
	cmdAttrs	= syscall.SysProcAttr{
		Pdeathsig:	syscall.SIGTERM,
	}
	showLogs	bool
)

const (
	repoUrl		string = "https://github.com/Kraftland/portable-arch.git"
)


// Client must initialize success / error channel!
type elevateRequest struct {
	cmdline		[]string
	timeout		time.Duration
	wd		string
	err		chan error

	// If true, will return pipes for stdin, stdout, stderr in pipeChan
	wantPipe	bool

	pipeChan	chan(cmdPipe)

}

type cmdPipe struct {
	//stdinPipe	*io.PipeWriter //io.WriteCloser

	// If not nil, pipes the output to it instead of sending to stdout
	stdoutPipe	*io.PipeReader //io.ReadCloser

	// If not nil, pipes the error to it instead of sending to stderr
	stderrPipe	*io.PipeReader //io.ReadCloser
}

// Struct to pass a repo info for a git package
type gitInfo struct {
	// Git URI
	uri		string
	defaultBranch	bool
	branch		string

}

type xdg struct {
	runtimeDir		string
	confDir			string
	cacheDir		string
	dataDir			string
	home			string
	stateHome		string
}

type pkgConf struct {
	// Arrays of tables containing dependencies not from core/extra
	Depends		[]DependsSection
	Metadata	confMeta
}

type confMeta struct {
	// The build prefix. Defaults to "extra-x86_64-build".
	BuildPrefix	string
	// GitHub user name of Maintainer
	Maintainer	string
	Type		string
	Repo		string
}

type DependsSection struct {
	Pkgname		string
	// Source can be either a string of git URL (type git), or repo name (type repo) to download from locally defined repositories.
	SourceType	string
	Source		string
	Branch		string
	// The build prefix for type git. Defaults to "extra-x86_64-build". Note that this might be empty.
	BuildPrefix	string
	Install		bool
	Depends		[]DependsSection
}

type envConf struct {
	elevateProgram		string
}

// Type pkginfo describes the information of a package file
type pkginfo struct {
	// pkgname describes the path for a package
	pkgname		string
	install		bool
}

// This type describes a locally installed package
type installedPackage struct {
	name		string // pkgname
	installedVer	string
	repoVer		string
	hasUpdate	bool
}