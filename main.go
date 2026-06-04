package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/grimdork/climate/arg"
	ll "github.com/grimdork/loglines"
)

var version = "dev"

func main() {
	opt := arg.New("devil", "Watch a binary and restart it when recompiled.")
	opt.SetDefaultHelp(true)
	opt.SetOption(arg.GroupDefault, "V", "version", "Print version and exit.", "", false, arg.VarBool, nil)
	opt.SetOption(arg.GroupDefault, "e", "envfile", "File containing environment variable key-value pairs.", "", false, arg.VarString, nil)
	opt.SetPositional("PROGRAM", "Program to run and keep running.", "", true, arg.VarString)
	opt.SetPositional("ARGS", "Program arguments.", "", false, arg.VarStringSlice)
	m := ll.Msg
	e := ll.Err

	args := os.Args[1:]
	err := opt.Parse(args)
	if err != nil {
		if err == arg.ErrNoArgs || err == arg.ErrNonFatal {
			opt.PrintHelp()
			return
		}
		e("Error: %s", err.Error())
		os.Exit(2)
	}

	if opt.GetBool("version") {
		fmt.Printf("devil version %s\n", version)
		return
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

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	quit := make(chan struct{})
	restart := make(chan struct{}, 1)

	pargs := opt.GetPosStringSlice("ARGS")
	m("Watching %s running with arguments '%s'", program, strings.Join(pargs, " "))

	cmd, err := runServer(program, pargs, env)
	if err != nil {
		if isNotFound(err) {
			m("Binary '%s' not found, retrying every 10s", program)
			cmd = waitForBinary(program, pargs, env, quit, m, e)
			if cmd == nil {
				return
			}
		} else {
			e("Couldn't start process '%s': %s", program, err.Error())
			os.Exit(2)
		}
	}

	go watcher(w, quit, restart, e)

	for {
		select {
		case <-restart:
			stopProcess(cmd, e)
			cmd, err = runServer(program, pargs, env)
			if err != nil {
				if isNotFound(err) {
					m("Binary '%s' not found, retrying every 10s", program)
					cmd = waitForBinary(program, pargs, env, quit, m, e)
					if cmd == nil {
						return
					}
				} else {
					e("Couldn't start process '%s': %s", program, err.Error())
					continue
				}
			}
			m("Restarted %s", program)
		case sig := <-sigCh:
			m("Received %s, shutting down", sig)
			if cmd != nil && cmd.Process != nil {
				syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
			}
			stopProcess(cmd, e)
			return
		case <-quit:
			return
		}
	}
}

func isNotFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound) || os.IsNotExist(err)
}

func waitForBinary(program string, pargs, env []string, quit <-chan struct{}, m, e func(string, ...interface{})) *exec.Cmd {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cmd, err := runServer(program, pargs, env)
			if err == nil {
				return cmd
			}
			if !isNotFound(err) {
				e("Error starting '%s': %s", program, err.Error())
			}
		case <-quit:
			return nil
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
	_ = cmd.Process.Signal(os.Interrupt)
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
		_ = cmd.Process.Kill()
		<-done
	}
}
