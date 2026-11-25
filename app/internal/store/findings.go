package store

import (
	"sync"
	"time"
)

// Finding is a manually recorded issue, like Burp's Organizer / Caido Findings.
type Finding struct {
	ID       int64     `json:"id"`
	Time     time.Time `json:"time"`
	Title    string    `json:"title"`
	Severity string    `json:"severity"` // info | low | medium | high | critical
	Host     string    `json:"host,omitempty"`
	FlowID   int64     `json:"flowId,omitempty"`
	Note     string    `json:"note,omitempty"`
}

// Findings is a thread-safe list of findings.
type Findings struct {
	mu     sync.RWMutex
	items  []*Finding
	nextID int64
}

// NewFindings creates an empty findings list.
func NewFindings() *Findings { return &Findings{nextID: 1} }

// Add appends a finding.
func (f *Findings) Add(x *Finding) *Finding {
	f.mu.Lock()
	defer f.mu.Unlock()
	x.ID = f.nextID
	f.nextID++
	if x.Time.IsZero() {
		x.Time = time.Now()
	}
	f.items = append(f.items, x)
	return x
}

// Remove deletes a finding by ID.
func (f *Findings) Remove(id int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.items[:0]
	for _, x := range f.items {
		if x.ID != id {
			out = append(out, x)
		}
	}
	f.items = out
}

// List returns all findings (newest first).
func (f *Findings) List() []*Finding {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]*Finding, 0, len(f.items))
	for i := len(f.items) - 1; i >= 0; i-- {
		out = append(out, f.items[i])
	}
	return out
}

// Snapshot returns findings oldest-first for export.
func (f *Findings) Snapshot() []*Finding {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]*Finding, len(f.items))
	copy(out, f.items)
	return out
}

// Load replaces the findings (used by project load).
func (f *Findings) Load(items []*Finding) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items = items
	var maxID int64
	for _, x := range items {
		if x.ID > maxID {
			maxID = x.ID
		}
	}
	f.nextID = maxID + 1
}
