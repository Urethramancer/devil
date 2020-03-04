package main

import (
	"os"
	"os/exec"
	"strings"

	"github.com/fsnotify/fsnotify"

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

	w, err := fsnotify.NewWatcher()
	if err != nil {
		e("Error creating watcher: %s", err.Error())
		os.Exit(2)
	}

	defer w.Close()
	app := os.Args[1]
	args := os.Args[2:]
	w.Add(app)

	m("Watching %s running with arguments '%s'", app, strings.Join(args, " "))
	cmd, err := runServer(app, args)
	if err != nil {
		e("Couldn't start process '%s': %s", app, err.Error())
		os.Exit(2)
	}

	ctrlc := daemon.BreakChannel()
	quit := make(chan bool)
	go func() {
		for {
			select {
			case ev := <-w.Events:
				switch {
				case ev.Op&fsnotify.Create == fsnotify.Create:
					cmd.Process.Signal(os.Interrupt)
					err = cmd.Wait()
					if err != nil {
						e("Error shutting down: %s", err.Error())
					}

					cmd, err = runServer(app, args)
					if err != nil {
						e("Couldn't start process '%s': %s", app, err.Error())
					}
				}
			case err := <-w.Errors:
				e("Watcher error: %s", err.Error())
			case <-ctrlc:
				cmd.Process.Signal(os.Interrupt)
				cmd.Wait()
				quit <- true
				return
			}
		}
	}()

	<-quit
}

func runServer(app string, args []string) (*exec.Cmd, error) {
	cmd := &exec.Cmd{}
	if len(args) > 0 {
		cmd = exec.Command(app, args...)
	} else {
		cmd = exec.Command(app)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	return cmd, err
}
