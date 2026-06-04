package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	ll "github.com/grimdork/loglines"
)

func runServer(app string, args, env []string) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if len(args) > 0 {
		cmd = exec.Command(app, args...)
	} else {
		cmd = exec.Command(app)
	}
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	path, err := filepath.Abs(app)
	if err != nil {
		ll.Err("Error resolving absolute path for '%s': %s", app, err.Error())
	}
	cmd.Dir = filepath.Dir(path)
	err = cmd.Start()
	return cmd, err
}
