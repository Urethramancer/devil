package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"

	ll "github.com/grimdork/loglines"
)

var signalMap = map[string]syscall.Signal{
	"hup":  syscall.SIGHUP,
	"usr1": syscall.SIGUSR1,
	"usr2": syscall.SIGUSR2,
	"quit": syscall.SIGQUIT,
}

// serve starts the HTTP server on addr. If the port is in use it logs a warning
// and returns without starting the server.
func serve(app *App, addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", handleStatus(app))
	mux.HandleFunc("/api/env", handleEnv(app))
	mux.HandleFunc("/api/env/load", handleEnvLoad(app))
	mux.HandleFunc("/api/env/add", handleEnvAdd(app))
	mux.HandleFunc("/api/env/remove/", handleEnvRemove(app))
	mux.HandleFunc("/api/env/apply", handleEnvApply(app))
	mux.HandleFunc("/api/events", handleEvents(app))
	mux.HandleFunc("/api/restart", handleRestart(app))
	mux.HandleFunc("/api/stop", handleStop(app))
	mux.HandleFunc("/api/quit", handleQuit(app))
	mux.HandleFunc("/api/signal/", handleSignal(app))
	mux.HandleFunc("/api/build", handleBuild(app))
	mux.HandleFunc("/api/pause", handlePause(app))
	mux.Handle("/", webHandler())

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		ll.Err("Web server: %s (running without web interface)", err.Error())
		return
	}

	go func() {
		<-app.quit
		srv.Close()
	}()

	ll.Msg("Web server: http://localhost%s", addr)
	srv.Serve(ln)
	ln.Close()
}

func handleStatus(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, app.Status())
	}
}

func handleEnv(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]interface{}{
			"current": app.Env(),
			"pending": app.PendingEnv(),
		})
	}
}

func handleEnvLoad(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := app.LoadEnvFile(req.Path); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		handleEnv(app)(w, r)
	}
}

func handleEnvAdd(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Key == "" {
			http.Error(w, "key is required", http.StatusBadRequest)
			return
		}
		app.AddPendingEnv(req.Key, req.Value)
		writeJSON(w, map[string]interface{}{
			"pending": app.PendingEnv(),
		})
	}
}

func handleEnvRemove(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		key, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/api/env/remove/"))
		if err != nil || key == "" {
			http.Error(w, "invalid key", http.StatusBadRequest)
			return
		}
		app.RemovePendingEnv(key)
		writeJSON(w, map[string]interface{}{
			"pending": app.PendingEnv(),
		})
	}
}

func handleEnvApply(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		go app.ApplyEnv()
		writeJSON(w, map[string]string{"status": "applying"})
	}
}

func handleEvents(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := app.logBuf.Subscribe()
		defer app.logBuf.Unsubscribe(ch)

		// Send backlog of last 200 lines
		for _, entry := range app.logBuf.Lines(200) {
			writeSSE(w, flusher, SSEEvent{
				Type: "log",
				Time: entry.Time,
				Line: entry.Line,
			})
		}

		for {
			select {
			case evt := <-ch:
				if err := writeSSE(w, flusher, evt); err != nil {
					return
				}
			case <-r.Context().Done():
				return
			}
		}
	}
}

func handleRestart(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		go app.Restart()
		writeJSON(w, map[string]string{"status": "restarting"})
	}
}

func handleStop(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		go app.Stop()
		writeJSON(w, map[string]string{"status": "stopped"})
	}
}

func handleQuit(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, map[string]string{"status": "shutting down"})
		go app.Quit()
	}
}

func handleSignal(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/api/signal/")
		sig, ok := signalMap[name]
		if !ok {
			http.Error(w, "unknown signal: "+name, http.StatusBadRequest)
			return
		}
		app.Signal(sig)
		writeJSON(w, map[string]string{"status": "signalled", "signal": name})
	}
}

func handleBuild(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Cmd string `json:"cmd"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.Cmd != "" {
			app.SetBuildCmd(req.Cmd)
		}
		go app.Build()
		writeJSON(w, map[string]string{"status": "building"})
	}
}

func handlePause(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		app.TogglePause()
		writeJSON(w, map[string]interface{}{
			"watching": app.Status()["watching"],
		})
	}
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, evt SSEEvent) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, string(data))
	if err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
