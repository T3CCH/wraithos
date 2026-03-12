package system

import (
	"fmt"
	"sync"
	"time"
)

const defaultBufferSize = 1000

// LogEntry represents a single log line.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
	Message  string    `json:"message"`
	Level    string    `json:"level"`
}

// RingBuffer is a thread-safe circular buffer for log entries.
type RingBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	size    int
	head    int
	count   int
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer(size int) *RingBuffer {
	if size <= 0 {
		size = defaultBufferSize
	}
	return &RingBuffer{
		entries: make([]LogEntry, size),
		size:    size,
	}
}

// Add appends a log entry to the ring buffer.
func (rb *RingBuffer) Add(entry LogEntry) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.entries[rb.head] = entry
	rb.head = (rb.head + 1) % rb.size
	if rb.count < rb.size {
		rb.count++
	}
}

// AddMessage is a convenience method to add a log entry from components.
func (rb *RingBuffer) AddMessage(source, level, message string) {
	rb.Add(LogEntry{
		Timestamp: time.Now(),
		Source:    source,
		Message:  message,
		Level:    level,
	})
}

// Entries returns all log entries in chronological order.
func (rb *RingBuffer) Entries() []LogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	result := make([]LogEntry, 0, rb.count)

	if rb.count < rb.size {
		// Buffer not yet full, entries start at 0
		for i := 0; i < rb.count; i++ {
			result = append(result, rb.entries[i])
		}
	} else {
		// Buffer is full, read from head (oldest) to head-1 (newest)
		for i := 0; i < rb.size; i++ {
			idx := (rb.head + i) % rb.size
			result = append(result, rb.entries[idx])
		}
	}

	return result
}

// Last returns the most recent n entries.
func (rb *RingBuffer) Last(n int) []LogEntry {
	all := rb.Entries()
	if n >= len(all) {
		return all
	}
	return all[len(all)-n:]
}

// Count returns the number of entries currently in the buffer.
func (rb *RingBuffer) Count() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}

// LogCollector aggregates logs from multiple sources into a ring buffer.
type LogCollector struct {
	buffer *RingBuffer
}

// NewLogCollector creates a log collector with the given buffer size.
func NewLogCollector(bufferSize int) *LogCollector {
	return &LogCollector{
		buffer: NewRingBuffer(bufferSize),
	}
}

// Log adds a message to the collector.
func (lc *LogCollector) Log(source, level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	lc.buffer.AddMessage(source, level, msg)
}

// Info logs an info-level message.
func (lc *LogCollector) Info(source, format string, args ...interface{}) {
	lc.Log(source, "info", format, args...)
}

// Warn logs a warning-level message.
func (lc *LogCollector) Warn(source, format string, args ...interface{}) {
	lc.Log(source, "warn", format, args...)
}

// Error logs an error-level message.
func (lc *LogCollector) Error(source, format string, args ...interface{}) {
	lc.Log(source, "error", format, args...)
}

// GetEntries returns all collected log entries.
func (lc *LogCollector) GetEntries() []LogEntry {
	return lc.buffer.Entries()
}

// GetLastN returns the most recent n log entries.
func (lc *LogCollector) GetLastN(n int) []LogEntry {
	return lc.buffer.Last(n)
}
