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
		m("Not enough arguments. Usage:\n%s <executable> [args...]", os.Args[0])
		os.Exit(1)
	}

	app := os.Args[1]
	args := os.Args[2:]
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
		e("Couldn't start process '%s': %s", app, err.Error())
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
