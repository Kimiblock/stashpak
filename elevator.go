package main

import (
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
)

func askRoot() error {
	cmd := exec.Command(
		conf.elevateProgram,
		"/usr/bin/true",
	)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func elevator(debug *log.Logger, warn *log.Logger) {
	var hasAsked bool
	var askLock sync.RWMutex

	for sig := range elevate {
		askLock.RLock()
		if ! hasAsked {
			askLock.RUnlock()
			askLock.Lock()
			err := askRoot()
			if err != nil {
				warn.Fatalln(
					"Could not ask for root privileges:",
					err,
				)
			}
			hasAsked = true
			askLock.Unlock()
		} else {
			askLock.RUnlock()
		}

		signal := sig
		go func () {
			var wd string
			if len(signal.wd) > 0 {
				wd = signal.wd
			} else {
				home, err := os.UserHomeDir()
				if err != nil {
					warn.Fatalln("Could not get user home:", err)
				}
				wd = home
			}

			debug.Println("Starting privileged command:", signal.cmdline)
			debug.Println("Using working directory:", wd)

			ctx := context.TODO()
			ctxTimeout, cancelFunc := context.WithTimeout(
				ctx,
				signal.timeout,
			)
			defer cancelFunc()
			var cmd *exec.Cmd

			if signal.timeout == 0 {
				cmd = exec.Command(
					conf.elevateProgram,
					signal.cmdline...,
				)
			} else {
				cmd = exec.CommandContext(
					ctxTimeout,
					conf.elevateProgram,
					signal.cmdline...,
				)
			}

			cmd.SysProcAttr = &cmdAttrs
			cmd.Dir = wd

			if signal.wantPipe {
				inR, inW := io.Pipe()
				cmd.Stdin = inR
				outR, outW := io.Pipe()
				cmd.Stdout = outW
				errR, errW := io.Pipe()
				cmd.Stderr = errW

				var pipes cmdPipe
				pipes.stdinPipe = inW
				pipes.stdoutPipe = outR
				pipes.stderrPipe = errR
				signal.pipeChan <- pipes
			} else {
				cmd.Stderr = os.Stderr
				cmd.Stdout = os.Stdout
			}

			err := cmd.Start()
			if err != nil {
				warn.Println("Elevated command has failed:", err)
				signal.err <- err
				return
			}
			cancelFunc()

			err = cmd.Wait()

			if err != nil {
				warn.Println("Elevated command has failed:", err)
				signal.err <- err
				return
			} else {
				signal.err <- nil
			}
		} ()
	}
}