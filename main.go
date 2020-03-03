package main

import (
	"os"
	"os/exec"

	"github.com/Urethramancer/daemon"

	"github.com/Urethramancer/signor/log"
)

func main() {
	m := log.Default.TMsg
	e := log.Default.TErr
	if len(os.Args) < 2 {
		e("Not enough arguments.")
		os.Exit(1)
	}

	app := os.Args[1]
	args := os.Args[2:]
	m("%s: %v", app, args)
	cmd := &exec.Cmd{}
	if len(args) > 0 {
		cmd = exec.Command(app, args...)
	} else {
		cmd = exec.Command(app)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	if err != nil {
		os.Exit(2)
	}

	ctrlc := daemon.BreakChannel()
	quit := make(chan bool)
	go func() {
		select {
		case <-ctrlc:
			cmd.Process.Signal(os.Interrupt)
			cmd.Wait()
			quit <- true
		}
	}()

	<-quit
}