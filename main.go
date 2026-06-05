package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/grimdork/climate/arg"
	ll "github.com/grimdork/loglines"
)

var version = "dev"

func main() {
	opt := arg.New("devil", "Watch a binary and restart it when recompiled.")
	opt.SetDefaultHelp(true)
	opt.SetOption(arg.GroupDefault, "V", "version", "Print version and exit.", false, false, arg.VarBool, nil)
	opt.SetOption(arg.GroupDefault, "p", "port", "Web server port.", "48128", false, arg.VarString, nil)
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

	program := opt.GetPosString("PROGRAM")
	pargs := opt.GetPosStringSlice("ARGS")
	port := opt.GetString("port")

	logBuf := NewLogBuffer(5000)

	app, err := NewApp(program, pargs, env, logBuf)
	if err != nil {
		e("Error: %s", err.Error())
		os.Exit(2)
	}

	go serve(app, ":"+port)

	m("Watching %s running with arguments '%s'", program, strings.Join(pargs, " "))

	if err := app.Start(); err != nil {
		if isNotFound(err) {
			m("Binary '%s' not found, retrying every 10s", program)
			go app.waitForBinary()
		} else {
			e("Couldn't start process '%s': %s", program, err.Error())
			os.Exit(2)
		}
	}

	app.MainLoop()
}
