package main

import (
	"os"
	"os/exec"
	"strings"

	"github.com/Urethramancer/daemon"
	"github.com/Urethramancer/signor/log"
	"github.com/Urethramancer/signor/opt"
	"github.com/fsnotify/fsnotify"
)

var o struct {
	opt.DefaultHelp
	Envfile string   `short:"e" long:"envfile" placeholder:"FILE" help:"File containing environment variable key-value pairs."`
	App     string   `placeholder:"PROGRAM" help:"Program to run and keep running."`
	Args    []string `placeholder:"ARGS" help:"Program arguments."`
}

func main() {
	m := log.Default.TMsg
	e := log.Default.TErr

	a := opt.Parse(&o)
	if o.Help {
		a.Usage()
		return
	}

	var env []string
	var err error
	if o.Envfile != "" {
		env, err = LoadEnv(o.Envfile)
		if err != nil {
			os.Exit(2)
		}
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		e("Error creating watcher: %s", err.Error())
		os.Exit(2)
	}

	defer w.Close()
	w.Add(o.App)

	ctrlc := daemon.BreakChannel()
	quit := make(chan bool)
	go func() {
		var err error
		var cmd *exec.Cmd

		m("Watching %s running with arguments '%s'", o.App, strings.Join(o.Args, " "))
		cmd, err = runServer(o.App, o.Args, env)
		if err != nil {
			e("Couldn't start process '%s': %s", o.App, err.Error())
			os.Exit(2)
		}
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

					cmd, err = runServer(o.App, o.Args, env)
					if err != nil {
						e("Couldn't start process '%s': %s", o.App, err.Error())
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
