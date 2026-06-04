package main

import (
	"io"
	"sync"
	"time"
)

// teeWriter always writes to both destinations, ignoring errors from the first.
type teeWriter struct {
	w1 io.Writer
	w2 io.Writer
}

func (t *teeWriter) Write(p []byte) (int, error) {
	n, err := t.w1.Write(p)
	t.w2.Write(p)
	return n, err
}

// LogEntry is a single timestamped log line.
type LogEntry struct {
	Time string `json:"time"`
	Line string `json:"line"`
}

// SSEEvent is sent over Server-Sent Events to the web UI.
// Different types ("log", "build-start", "build-end") are mapped
// to named SSE events so the client can dispatch them separately.
type SSEEvent struct {
	Type    string `json:"type"`
	Time    string `json:"time,omitempty"`
	Line    string `json:"line,omitempty"`
	Cmd     string `json:"cmd,omitempty"`
	Success bool   `json:"success,omitempty"`
	Output  string `json:"output,omitempty"`
}

// LogBuffer is a fixed-size ring buffer of timestamped log lines
// with SSE subscriber support.
type LogBuffer struct {
	mu  sync.RWMutex
	buf []LogEntry
	max int
	pos int

	subMu sync.RWMutex
	subs  map[chan SSEEvent]struct{}
}

// NewLogBuffer returns a ring buffer holding up to max entries.
func NewLogBuffer(max int) *LogBuffer {
	return &LogBuffer{
		buf:  make([]LogEntry, max),
		max:  max,
		subs: make(map[chan SSEEvent]struct{}),
	}
}

// Add appends a log line to the buffer and broadcasts it to SSE subscribers.
func (lb *LogBuffer) Add(line string) {
	entry := LogEntry{
		Time: time.Now().Format("15:04:05"),
		Line: line,
	}

	lb.mu.Lock()
	lb.buf[lb.pos%lb.max] = entry
	lb.pos++
	lb.mu.Unlock()

	lb.broadcast(SSEEvent{
		Type: "log",
		Time: entry.Time,
		Line: entry.Line,
	})
}

// Broadcast sends an arbitrary SSE event to all subscribers.
func (lb *LogBuffer) Broadcast(evt SSEEvent) {
	lb.broadcast(evt)
}

func (lb *LogBuffer) broadcast(evt SSEEvent) {
	lb.subMu.RLock()
	defer lb.subMu.RUnlock()
	for ch := range lb.subs {
		select {
		case ch <- evt:
		default:
		}
	}
}

// Subscribe returns a channel that receives all new log entries and broadcast events.
func (lb *LogBuffer) Subscribe() chan SSEEvent {
	ch := make(chan SSEEvent, 256)
	lb.subMu.Lock()
	lb.subs[ch] = struct{}{}
	lb.subMu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel.
func (lb *LogBuffer) Unsubscribe(ch chan SSEEvent) {
	lb.subMu.Lock()
	delete(lb.subs, ch)
	lb.subMu.Unlock()
}

// Lines returns the last n log entries (chronological order).
func (lb *LogBuffer) Lines(n int) []LogEntry {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	total := lb.pos
	if total > lb.max {
		total = lb.max
	}
	if n <= 0 || n > total {
		n = total
	}

	result := make([]LogEntry, n)
	start := lb.pos - n
	for i := 0; i < n; i++ {
		idx := (start + i) % lb.max
		result[i] = lb.buf[idx]
	}
	return result
}

// NewStdoutWriter returns an io.Writer that writes to os.Stdout and the log buffer.
func (lb *LogBuffer) NewStdoutWriter(w io.Writer) io.Writer {
	return &teeWriter{w1: w, w2: newLogWriter(lb)}
}

// NewStderrWriter returns an io.Writer that writes to os.Stderr and the log buffer.
func (lb *LogBuffer) NewStderrWriter(w io.Writer) io.Writer {
	return &teeWriter{w1: w, w2: newLogWriter(lb)}
}

// logWriter buffers partial writes and feeds complete lines to the LogBuffer.
type logWriter struct {
	lb        *LogBuffer
	remainder []byte
	mu        sync.Mutex
}

func newLogWriter(lb *LogBuffer) *logWriter {
	return &logWriter{lb: lb}
}

func (lw *logWriter) Write(p []byte) (n int, err error) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	n = len(p)
	data := lw.remainder
	lw.remainder = nil

	if len(data) > 0 {
		data = append(data, p...)
	} else {
		data = p
	}

	start := 0
	for i, b := range data {
		if b == '\n' {
			line := data[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lw.lb.Add(string(line))
			start = i + 1
		}
	}

	if start < len(data) {
		lw.remainder = make([]byte, len(data)-start)
		copy(lw.remainder, data[start:])
	}

	return n, nil
}
