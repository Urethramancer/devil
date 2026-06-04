package main

import (
	"os"
	"os/exec"
	"strings"
	"time"

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

	args := os.Args[1:]
	err := opt.Parse(args)
	if err != nil {
		if err == arg.ErrNoArgs {
			opt.PrintHelp()
			return
		}
		e("Error: %s", err.Error())
		os.Exit(2)
	}

	envfile := opt.GetString("envfile")
	var env []string
	if envfile != "" {
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
	if err = w.Add(program); err != nil {
		e("Error watching '%s': %s", program, err.Error())
		os.Exit(2)
	}

	ctrlc := daemon.BreakChannel()
	quit := make(chan struct{})
	restart := make(chan struct{}, 1)

	pargs := opt.GetPosStringSlice("ARGS")
	m("Watching %s running with arguments '%s'", program, strings.Join(pargs, " "))

	cmd, err := runServer(program, pargs, env)
	if err != nil {
		e("Couldn't start process '%s': %s", program, err.Error())
		os.Exit(2)
	}

	go watcher(w, quit, restart, e)

	for {
		select {
		case <-restart:
			stopProcess(cmd, e)
			cmd, err = runServer(program, pargs, env)
			if err != nil {
				e("Couldn't start process '%s': %s", program, err.Error())
				continue
			}
			m("Restarted %s", program)
		case <-ctrlc:
			stopProcess(cmd, e)
			return
		case <-quit:
			return
		}
	}
}

func watcher(w *fsnotify.Watcher, quit chan<- struct{}, restart chan<- struct{}, e func(string, ...interface{})) {
	defer func() {
		if r := recover(); r != nil {
			e("Watcher panic: %v", r)
		}
		close(quit)
	}()

	var timer *time.Timer
	for {
		select {
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(200*time.Millisecond, func() {
					select {
					case restart <- struct{}{}:
					default:
					}
				})
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			e("Watcher error: %s", err.Error())
		}
	}
}

func stopProcess(cmd *exec.Cmd, e func(string, ...interface{})) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		if err != nil {
			e("Error shutting down: %s", err.Error())
		}
	case <-time.After(5 * time.Second):
		e("Process did not exit gracefully, killing")
		cmd.Process.Kill()
		<-done
	}
}
