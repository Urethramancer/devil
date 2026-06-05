package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	ll "github.com/grimdork/loglines"
)

// App holds the shared state for the process supervisor and web server.
type App struct {
	program string
	pargs   []string

	mu         sync.RWMutex
	restartMu  sync.Mutex
	cmd        *exec.Cmd
	env        []string // applied environment
	pendingEnv []string // not yet applied
	startTime  time.Time
	restarts   int
	running    bool
	watching   bool

	w       *fsnotify.Watcher
	quit    chan struct{}
	restart chan struct{}

	buildCmd string

	logBuf       *LogBuffer
	stdoutWriter io.Writer
	stderrWriter io.Writer
}

// NewApp creates a new App and attaches the filesystem watcher to the given binary.
func NewApp(program string, pargs, env []string, logBuf *LogBuffer) (*App, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err = w.Add(program); err != nil {
		w.Close()
		return nil, err
	}

	pending := make([]string, len(env))
	copy(pending, env)

	envCopy := make([]string, len(env))
	copy(envCopy, env)

	pargsCopy := make([]string, len(pargs))
	copy(pargsCopy, pargs)

	app := &App{
		program:      program,
		pargs:        pargsCopy,
		env:          envCopy,
		pendingEnv:   pending,
		w:            w,
		watching:     true,
		quit:         make(chan struct{}),
		restart:      make(chan struct{}, 1),
		logBuf:       logBuf,
		stdoutWriter: logBuf.NewStdoutWriter(os.Stdout),
		stderrWriter: logBuf.NewStderrWriter(os.Stderr),
	}
	return app, nil
}

// Start launches the watched process.
func (app *App) Start() error {
	app.mu.Lock()
	defer app.mu.Unlock()

	if app.cmd != nil {
		return errors.New("process already running")
	}

	var cmd *exec.Cmd
	if len(app.pargs) > 0 {
		cmd = exec.Command(app.program, app.pargs...)
	} else {
		cmd = exec.Command(app.program)
	}
	cmd.Env = app.env
	cmd.Stdout = app.stdoutWriter
	cmd.Stderr = app.stderrWriter
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	path, err := filepath.Abs(app.program)
	if err != nil {
		ll.Err("Error resolving absolute path for '%s': %s", app.program, err.Error())
	}
	cmd.Dir = filepath.Dir(path)
	if err = cmd.Start(); err != nil {
		return err
	}

	app.cmd = cmd
	app.startTime = time.Now()
	app.running = true
	app.restarts++
	return nil
}

// Stop sends SIGINT to the child and waits up to 5 seconds before SIGKILL.
func (app *App) Stop() {
	app.mu.Lock()
	cmd := app.cmd
	app.running = false
	app.cmd = nil
	app.mu.Unlock()

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
			ll.Err("Error shutting down: %s", err.Error())
		}
	case <-time.After(5 * time.Second):
		ll.Err("Process did not exit gracefully, killing")
		_ = cmd.Process.Kill()
		<-done
	}
}

// Restart stops the current process and starts a new one.
func (app *App) Restart() {
	app.restartMu.Lock()
	defer app.restartMu.Unlock()
	app.Stop()
	if err := app.Start(); err != nil {
		if isNotFound(err) {
			ll.Msg("Binary '%s' not found, retrying every 10s", app.program)
			go app.waitForBinary()
		} else {
			ll.Err("Couldn't start process '%s': %s", app.program, err.Error())
		}
		return
	}
	ll.Msg("Restarted %s", app.program)
}

// waitForBinary polls until the watched binary exists, then triggers a restart.
func (app *App) waitForBinary() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if _, err := os.Stat(app.program); err == nil {
				select {
				case app.restart <- struct{}{}:
				default:
				}
				return
			}
		case <-app.quit:
			return
		}
	}
}

// Signal sends a signal to the child process group.
func (app *App) Signal(sig syscall.Signal) {
	app.mu.RLock()
	cmd := app.cmd
	app.mu.RUnlock()
	if cmd != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, sig)
	}
}

// TogglePause enables or disables file watching.
func (app *App) TogglePause() {
	app.mu.Lock()
	app.watching = !app.watching
	app.mu.Unlock()
}

// Quit shuts down the watcher and signals the main loop to exit.
func (app *App) Quit() {
	select {
	case <-app.quit:
	default:
		close(app.quit)
	}
}

// --- Environment management ---

// Env returns the applied environment.
func (app *App) Env() []string {
	app.mu.RLock()
	defer app.mu.RUnlock()
	out := make([]string, len(app.env))
	copy(out, app.env)
	return out
}

// PendingEnv returns the pending (not yet applied) environment.
func (app *App) PendingEnv() []string {
	app.mu.RLock()
	defer app.mu.RUnlock()
	out := make([]string, len(app.pendingEnv))
	copy(out, app.pendingEnv)
	return out
}

// SetPendingEnv replaces the pending env list.
func (app *App) SetPendingEnv(vars []string) {
	app.mu.Lock()
	app.pendingEnv = make([]string, len(vars))
	copy(app.pendingEnv, vars)
	app.mu.Unlock()
}

// AddPendingEnv adds a single key=value pair to the pending list.
func (app *App) AddPendingEnv(k, v string) {
	app.mu.Lock()
	app.pendingEnv = append(app.pendingEnv, k+"="+v)
	app.mu.Unlock()
}

// RemovePendingEnv removes the first pending var matching the key prefix.
func (app *App) RemovePendingEnv(key string) {
	app.mu.Lock()
	defer app.mu.Unlock()
	prefix := key + "="
	for i, e := range app.pendingEnv {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			app.pendingEnv = append(app.pendingEnv[:i], app.pendingEnv[i+1:]...)
			return
		}
	}
}

// ApplyEnv copies pending to applied and restarts the process.
func (app *App) ApplyEnv() {
	app.mu.Lock()
	app.env = make([]string, len(app.pendingEnv))
	copy(app.env, app.pendingEnv)
	app.mu.Unlock()
	app.Restart()
}

// LoadEnvFile loads an env file and sets it as pending.
func (app *App) LoadEnvFile(path string) error {
	vars, err := LoadEnv(path)
	if err != nil {
		return err
	}
	app.SetPendingEnv(vars)
	return nil
}

// EnvEntry holds a single environment variable for the web API.
type EnvEntry struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Masked bool   `json:"masked"`
}

var sensitivePatterns = []string{
	"KEY", "PASSWORD", "PASSWD", "SECRET", "TOKEN",
	"AWS_ACCESS", "AWS_SECRET", "CREDENTIAL", "PRIVATE",
	"AUTH", "SIGNING", "CERT", "CERTIFICATE",
	"API_KEY", "ACCESS_KEY", "ENCRYPT",
}

// isSensitive returns true if the key name matches common credential patterns.
func isSensitive(key string) bool {
	upper := strings.ToUpper(key)
	for _, p := range sensitivePatterns {
		if strings.Contains(upper, p) {
			return true
		}
	}
	return false
}

func envEntries(vars []string) []EnvEntry {
	entries := make([]EnvEntry, len(vars))
	for i, e := range vars {
		eq := strings.IndexByte(e, '=')
		if eq < 0 {
			entries[i] = EnvEntry{Key: e, Value: "", Masked: false}
			continue
		}
		key := e[:eq]
		val := e[eq+1:]
		entries[i] = EnvEntry{
			Key:    key,
			Value:  val,
			Masked: isSensitive(key),
		}
	}
	return entries
}

// EnvEntries returns the applied environment as structured entries.
func (app *App) EnvEntries() []EnvEntry {
	app.mu.RLock()
	defer app.mu.RUnlock()
	return envEntries(app.env)
}

// PendingEnvEntries returns the pending environment as structured entries.
func (app *App) PendingEnvEntries() []EnvEntry {
	app.mu.RLock()
	defer app.mu.RUnlock()
	return envEntries(app.pendingEnv)
}

// Build runs the configured build command, streams output to the log,
// and restarts on success.
func (app *App) Build() {
	app.mu.RLock()
	buildCmd := app.buildCmd
	app.mu.RUnlock()

	if buildCmd == "" {
		ll.Err("No build command configured")
		return
	}

	app.logBuf.Broadcast(SSEEvent{
		Type: "build-start",
		Cmd:  buildCmd,
	})

	cmd := exec.Command("sh", "-c", buildCmd)
	cmd.Env = app.Env()
	cmd.Stdout = app.stdoutWriter
	cmd.Stderr = app.stderrWriter

	err := cmd.Run()
	success := err == nil

	output := ""
	if !success {
		output = err.Error()
	}

	app.logBuf.Broadcast(SSEEvent{
		Type:    "build-end",
		Success: success,
		Output:  output,
	})

	if success {
		app.Restart()
	} else {
		ll.Err("Build failed: %s", err.Error())
	}
}

// --- Read-only accessors for the web API ---

// Status returns a snapshot of the current state.
func (app *App) Status() map[string]interface{} {
	app.mu.RLock()
	defer app.mu.RUnlock()

	uptime := ""
	if !app.startTime.IsZero() && app.running {
		uptime = time.Since(app.startTime).Round(time.Second).String()
	}

	pid := 0
	if app.cmd != nil && app.cmd.Process != nil {
		pid = app.cmd.Process.Pid
	}

	return map[string]interface{}{
		"pid":      pid,
		"uptime":   uptime,
		"running":  app.running,
		"watching": app.watching,
		"restarts": app.restarts,
		"program":  app.program,
		"args":     app.pargs,
	}
}

// BuildCmd returns the stored build command.
func (app *App) BuildCmd() string {
	app.mu.RLock()
	defer app.mu.RUnlock()
	return app.buildCmd
}

// SetBuildCmd stores the build command.
func (app *App) SetBuildCmd(s string) {
	app.mu.Lock()
	app.buildCmd = s
	app.mu.Unlock()
}

// Watcher runs the fsnotify event loop in a goroutine.
func (app *App) Watcher() {
	defer func() {
		if r := recover(); r != nil {
			ll.Err("Watcher panic: %v", r)
		}
		app.Quit()
	}()

	var timer *time.Timer
	for {
		select {
		case ev, ok := <-app.w.Events:
			if !ok {
				return
			}
			if !app.isWatching() {
				continue
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(200*time.Millisecond, func() {
					select {
					case app.restart <- struct{}{}:
					default:
					}
				})
			}
		case err, ok := <-app.w.Errors:
			if !ok {
				return
			}
			ll.Err("Watcher error: %s", err.Error())
		case <-app.quit:
			return
		}
	}
}

func (app *App) isWatching() bool {
	app.mu.RLock()
	defer app.mu.RUnlock()
	return app.watching
}

// isNotFound reports whether err indicates the executable was not found.
func isNotFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound) || os.IsNotExist(err)
}

// MainLoop is the primary event loop. Blocks until shutdown.
func (app *App) MainLoop() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go app.Watcher()

	for {
		select {
		case <-app.restart:
			app.Restart()
		case sig := <-sigCh:
			ll.Msg("Received %s, shutting down", sig)
			app.mu.RLock()
			cmd := app.cmd
			app.mu.RUnlock()
			if cmd != nil && cmd.Process != nil {
				syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
			}
			app.Stop()
			return
		case <-app.quit:
			return
		}
	}
}
