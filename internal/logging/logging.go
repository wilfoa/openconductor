// Package logging provides a file-based structured logger for OpenConductor.
//
// Since the TUI owns stdout/stderr, all diagnostic output goes to a log file
// at ~/.openconductor/openconductor.log. The package uses stdlib log/slog for
// zero-dep structured logging with JSON output.
//
// Usage:
//
//	logging.Init(logging.Options{Debug: true})
//	defer logging.Close()
//
//	logging.Info("session started", "project", name, "agent", agent)
//	logging.Error("PTY read failed", "err", err)
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"
)

const (
	// defaultLogFile is the log filename within the openconductor config dir.
	defaultLogFile = "openconductor.log"

	// maxLogSize is the approximate max size before rotation (5 MB).
	maxLogSize = 5 * 1024 * 1024
)

// Options configures the logger.
type Options struct {
	// Debug enables debug-level messages. Default is info-level.
	Debug bool

	// Dir overrides the log directory. Defaults to ~/.openconductor.
	Dir string
}

var (
	mu      sync.Mutex
	logFile *os.File
	logger  *slog.Logger
)

// Init sets up the global file logger. Call Close() on shutdown.
func Init(opts Options) error {
	mu.Lock()
	defer mu.Unlock()

	dir := opts.Dir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("logging: cannot determine home dir: %w", err)
		}
		dir = filepath.Join(home, ".openconductor")
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("logging: cannot create log dir: %w", err)
	}

	logPath := filepath.Join(dir, defaultLogFile)

	// Rotate if existing log is too large.
	if info, err := os.Stat(logPath); err == nil && info.Size() > maxLogSize {
		old := logPath + ".old"
		os.Remove(old)
		os.Rename(logPath, old)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("logging: cannot open log file: %w", err)
	}

	level := slog.LevelInfo
	if opts.Debug {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: level,
	})

	logFile = f
	logger = slog.New(handler)

	// Write a startup marker.
	logger.Info("openconductor starting",
		"pid", os.Getpid(),
		"debug", opts.Debug,
		"log_path", logPath,
	)

	return nil
}

// Close flushes and closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		logFile.Sync()
		logFile.Close()
		logFile = nil
	}
	logger = nil
}

// Writer returns an io.Writer that writes to the log file, useful for
// redirecting other loggers or capturing panic output. Returns io.Discard
// if the logger is not initialized.
func Writer() io.Writer {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		return logFile
	}
	return io.Discard
}

// ── Convenience functions ───────────────────────────────────────

// Debug logs a debug-level message (only visible with --debug flag).
func Debug(msg string, args ...any) {
	if l := get(); l != nil {
		l.Debug(msg, args...)
	}
}

// Info logs an info-level message.
func Info(msg string, args ...any) {
	if l := get(); l != nil {
		l.Info(msg, args...)
	}
}

// Warn logs a warning-level message.
func Warn(msg string, args ...any) {
	if l := get(); l != nil {
		l.Warn(msg, args...)
	}
}

// Error logs an error-level message.
func Error(msg string, args ...any) {
	if l := get(); l != nil {
		l.Error(msg, args...)
	}
}

// Fatal logs an error-level message and exits.
func Fatal(msg string, args ...any) {
	if l := get(); l != nil {
		l.Error(msg, args...)
	}
	Close()
	os.Exit(1)
}

// RecoverPanic should be deferred at the top of main(). It catches panics,
// logs the stack trace to the log file, then re-panics so the user sees it.
func RecoverPanic() {
	if r := recover(); r != nil {
		stack := string(debug.Stack())
		timestamp := time.Now().Format(time.RFC3339)

		// Log structured entry if logger is available.
		if l := get(); l != nil {
			l.Error("PANIC",
				"recovered", fmt.Sprintf("%v", r),
				"stack", stack,
			)
		}

		// Also write a human-readable crash report to the log file.
		if w := Writer(); w != io.Discard {
			fmt.Fprintf(w, "\n=== CRASH at %s ===\npanic: %v\n\n%s\n", timestamp, r, stack)
		}

		Close()

		// Re-panic so the user sees something on their terminal.
		panic(r)
	}
}

func get() *slog.Logger {
	mu.Lock()
	defer mu.Unlock()
	return logger
}
