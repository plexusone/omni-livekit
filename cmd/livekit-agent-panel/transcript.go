// Package main provides a panel discussion agent with multiple AI panelists.
package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Entry represents a single turn in the discussion.
type Entry struct {
	Speaker   string // "Moderator", "Alex", etc.
	Text      string
	Timestamp time.Time
}

// Transcript maintains the shared conversation history.
type Transcript struct {
	entries []Entry
	mu      sync.RWMutex
}

// NewTranscript creates a new empty transcript.
func NewTranscript() *Transcript {
	return &Transcript{
		entries: make([]Entry, 0),
	}
}

// Add appends a new entry to the transcript.
func (t *Transcript) Add(speaker, text string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.entries = append(t.entries, Entry{
		Speaker:   speaker,
		Text:      text,
		Timestamp: time.Now(),
	})
}

// Format returns the transcript formatted for LLM context.
func (t *Transcript) Format() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.entries) == 0 {
		return "(No discussion yet)"
	}

	var sb strings.Builder
	for _, entry := range t.entries {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", entry.Speaker, entry.Text))
	}
	return sb.String()
}

// Len returns the number of entries in the transcript.
func (t *Transcript) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.entries)
}

// LastN returns the last n entries formatted as a string.
// Useful for keeping context manageable for LLMs.
func (t *Transcript) LastN(n int) string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if len(t.entries) == 0 {
		return "(No discussion yet)"
	}

	start := 0
	if len(t.entries) > n {
		start = len(t.entries) - n
	}

	var sb strings.Builder
	for _, entry := range t.entries[start:] {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", entry.Speaker, entry.Text))
	}
	return sb.String()
}

// Clear removes all entries from the transcript.
func (t *Transcript) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = t.entries[:0]
}
