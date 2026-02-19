package terminal

import (
	"sync"
	"time"
)

// Buffer is a thread-safe container for terminal screen lines that tracks
// whether the content has changed since last inspection. It is designed to
// be updated frequently by a terminal emulator and read periodically by the
// attention detector.
type Buffer struct {
	lines      []string
	changed    bool
	lastChange time.Time
	mu         sync.RWMutex
}

// NewBuffer creates an empty Buffer ready for use.
func NewBuffer() *Buffer {
	return &Buffer{}
}

// Update replaces the buffer contents with new terminal screen lines and
// marks the buffer as changed.
func (b *Buffer) Update(lines []string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.lines = make([]string, len(lines))
	copy(b.lines, lines)
	b.changed = true
	b.lastChange = time.Now()
}

// LastLines returns the last n lines from the buffer. If n exceeds the
// number of stored lines, all available lines are returned. The returned
// slice is a copy and safe to use without holding the lock.
func (b *Buffer) LastLines(n int) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.lines) == 0 {
		return nil
	}

	if n > len(b.lines) {
		n = len(b.lines)
	}

	start := len(b.lines) - n
	result := make([]string, n)
	copy(result, b.lines[start:])
	return result
}

// HasChanged reports whether the buffer contents have been updated since
// the last call to MarkChecked.
func (b *Buffer) HasChanged() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.changed
}

// MarkChecked clears the changed flag, indicating that the current contents
// have been inspected by the attention detector.
func (b *Buffer) MarkChecked() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.changed = false
}

// TimeSinceChange returns the duration since the buffer was last updated.
// If the buffer has never been updated, it returns zero.
func (b *Buffer) TimeSinceChange() time.Duration {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.lastChange.IsZero() {
		return 0
	}
	return time.Since(b.lastChange)
}
