package main

import (
	"log"
	"context"
	"time"
	"os"
	"os/exec"
)

func elevator(debug *log.Logger, warn *log.Logger) {
	for sig := range elevate {
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

			if signal.timeout == 0 {
				cmd := exec.Command(conf.elevateProgram, signal.cmdline...)
				cmd.SysProcAttr = &cmdAttrs
				cmd.Dir = wd
				cmd.Stderr = os.Stderr
				cmd.Stdout = os.Stdout

				err := cmd.Run()
				if err != nil {
					warn.Println("Elevated command has failed:", err)
					signal.err <- err
				} else {
					signal.err <- nil
				}
			} else {
				ctx := context.TODO()
				ctxTimeout, cancelFunc := context.WithTimeout(ctx, signal.timeout)
				cmd := exec.CommandContext(ctxTimeout, conf.elevateProgram, signal.cmdline...)
				cmd.SysProcAttr = &cmdAttrs
				cmd.Stderr = os.Stderr
				cmd.Stdout = os.Stdout
				cmd.Dir = wd
				err := cmd.Run()
				cancelFunc()
				if err != nil {
					warn.Println("Elevated command has failed:", err)
					signal.err <- err
				} else {
					signal.err <- nil
				}
			}
		} ()
	}
}