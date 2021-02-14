package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

func runServer(app string, args, env []string) (*exec.Cmd, error) {
	cmd := &exec.Cmd{}
	if len(args) > 0 {
		cmd = exec.Command(app, args...)
	} else {
		cmd = exec.Command(app)
	}
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	path, _ := filepath.Abs(app)
	cmd.Dir = filepath.Dir(path)
	err := cmd.Start()
	return cmd, err
}
