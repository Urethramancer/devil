package main

import (
	"fmt"
	"os"
	"strings"
)

// LoadEnv loads environment variables from a file.
func LoadEnv(fn string) ([]string, error) {
	buf, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(buf), "\n")
	env := []string{}
	for _, x := range lines {
		if strings.Index(x, "=") == -1 {
			continue
		}

		a := strings.SplitN(x, "=", 2)
		k := strings.TrimSpace(a[0])
		v := strings.TrimSpace(a[1])
		e := fmt.Sprintf("%s=%s", k, v)
		env = append(env, e)
	}
	return env, nil
}
