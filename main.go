package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/Urethramancer/daemon"
	"github.com/fsnotify/fsnotify"
	"github.com/grimdork/climate/arg"
	ll "github.com/grimdork/loglines"
)

func main() {
	opt := arg.New("devil")
	opt.SetDefaultHelp(true)
	opt.SetOption(arg.GroupDefault, "e", "envfile", "File containing environment variable key-value pairs.", "", false, arg.VarString, nil)
	opt.SetPositional("PROGRAM", "Program to run and keep running.", "", true, arg.VarString)
	opt.SetPositional("ARGS", "Program arguments.", "", false, arg.VarStringSlice)
	m := ll.Msg
	e := ll.Err
	var env []string
	var err error
	fmt.Printf("Args: %#v\n", os.Args[1:])
	args := os.Args[1:]
	fmt.Printf("Args: %#v\n", args)
	err = opt.Parse(args)
	if err != nil {
		if err == arg.ErrNoArgs {
			opt.PrintHelp()
			return
		}

		fmt.Printf("Error: %s\n", err.Error())
		os.Exit(2)
	}

	envfile := opt.GetString("envfile")
	count := 2
	if envfile != "" {
		count++
		env, err = LoadEnv(envfile)
		if err != nil {
			e("Error loading environment file '%s': %s", envfile, err.Error())
			os.Exit(2)
		}
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		e("Error creating watcher: %s", err.Error())
		os.Exit(2)
	}

	defer w.Close()
	program := opt.GetPosString("PROGRAM")
	w.Add(program)

	ctrlc := daemon.BreakChannel()
	quit := make(chan bool)
	pargs := opt.GetPosStringSlice("ARGS")
	fmt.Printf("pargs: %#v\n", pargs)
	go func() {
		var err error
		var cmd *exec.Cmd

		m("Watching %s running with arguments '%s'", program, strings.Join(pargs, " "))
		cmd, err = runServer(program, pargs, env)
		if err != nil {
			e("Couldn't start process '%s': %s", program, err.Error())
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

					cmd, err = runServer(program, pargs, env)
					if err != nil {
						e("Couldn't start process '%s': %s", program, err.Error())
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
