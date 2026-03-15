//go:build e2e

package e2e

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// LogEntry represents a single JSON log line from openconductor.log.
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Msg     string `json:"msg"`
	Session string `json:"session,omitempty"`
	Project string `json:"project,omitempty"`
	raw     map[string]any
}

// Get returns the value of an arbitrary field from the log entry.
func (e *LogEntry) Get(key string) any {
	return e.raw[key]
}

// GetString returns a string field value, or "" if missing.
func (e *LogEntry) GetString(key string) string {
	v, _ := e.raw[key].(string)
	return v
}

// GetBool returns a bool field value, or false if missing.
func (e *LogEntry) GetBool(key string) bool {
	v, _ := e.raw[key].(bool)
	return v
}

// GetFloat returns a float64 field value, or 0 if missing.
func (e *LogEntry) GetFloat(key string) float64 {
	v, _ := e.raw[key].(float64)
	return v
}

// LogWatcher tails the JSON log file and provides methods to search and
// wait for specific log entries.
type LogWatcher struct {
	t       *testing.T
	path    string
	entries []LogEntry
	mu      sync.Mutex
	stop    chan struct{}
	done    chan struct{}
}

// NewLogWatcher starts watching the given log file.
func NewLogWatcher(t *testing.T, path string) *LogWatcher {
	w := &LogWatcher{
		t:    t,
		path: path,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go w.tailLoop()
	return w
}

func (w *LogWatcher) tailLoop() {
	defer close(w.done)

	// Wait for the file to exist.
	for {
		select {
		case <-w.stop:
			return
		default:
		}
		if _, err := os.Stat(w.path); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	f, err := os.Open(w.path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for {
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var entry LogEntry
			var raw map[string]any
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				continue
			}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue
			}
			entry.raw = raw

			w.mu.Lock()
			w.entries = append(w.entries, entry)
			w.mu.Unlock()
		}

		select {
		case <-w.stop:
			return
		default:
		}

		time.Sleep(50 * time.Millisecond)

		// Re-create scanner to read new data from current file position.
		scanner = bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	}
}

// Stop terminates the tail loop.
func (w *LogWatcher) Stop() {
	close(w.stop)
	<-w.done
}

// Entries returns a snapshot of all log entries seen so far.
func (w *LogWatcher) Entries() []LogEntry {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]LogEntry, len(w.entries))
	copy(out, w.entries)
	return out
}

// Count returns the current entry count (useful as a "since" marker).
func (w *LogWatcher) Count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.entries)
}

// FindEntry returns the first entry matching the predicate, searching from
// the given start index. Returns (entry, index, found).
func (w *LogWatcher) FindEntry(from int, pred func(LogEntry) bool) (LogEntry, int, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i := from; i < len(w.entries); i++ {
		if pred(w.entries[i]) {
			return w.entries[i], i, true
		}
	}
	return LogEntry{}, len(w.entries), false
}

// WaitForEntry polls until a log entry matching the predicate appears,
// or the timeout expires.
func (w *LogWatcher) WaitForEntry(timeout time.Duration, pred func(LogEntry) bool, failMsg string) LogEntry {
	w.t.Helper()
	deadline := time.Now().Add(timeout)
	from := 0

	for time.Now().Before(deadline) {
		entry, idx, found := w.FindEntry(from, pred)
		if found {
			return entry
		}
		from = idx
		time.Sleep(100 * time.Millisecond)
	}

	// Dump last 20 log entries for debugging.
	w.mu.Lock()
	recent := w.entries
	if len(recent) > 20 {
		recent = recent[len(recent)-20:]
	}
	w.mu.Unlock()

	var details strings.Builder
	for _, e := range recent {
		details.WriteString("  ")
		details.WriteString(e.Msg)
		if e.Session != "" {
			details.WriteString(" [")
			details.WriteString(e.Session)
			details.WriteString("]")
		}
		details.WriteString("\n")
	}

	w.t.Fatalf("timeout waiting for log entry: %s\nLast %d log entries:\n%s",
		failMsg, len(recent), details.String())
	return LogEntry{}
}
