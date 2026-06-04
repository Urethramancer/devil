package main

import (
	"fmt"
	"os"
	"strings"
)

// LoadEnv loads environment variables from a file.
// Lines starting with # or ; are treated as comments and skipped.
// Blank lines are skipped. Keys preserve their original case.
func LoadEnv(fn string) ([]string, error) {
	buf, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(buf), "\n")
	env := []string{}
	for _, x := range lines {
		x = strings.TrimSpace(x)
		if x == "" || strings.HasPrefix(x, "#") || strings.HasPrefix(x, ";") {
			continue
		}
		if !strings.Contains(x, "=") {
			continue
		}

		a := strings.SplitN(x, "=", 2)
		k := strings.TrimSpace(a[0])
		v := strings.TrimSpace(a[1])
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env, nil
}
