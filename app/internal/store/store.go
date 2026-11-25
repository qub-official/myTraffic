// Package store keeps the history of captured request/response flows. It holds
// flows in memory with a ring-buffer cap and optionally mirrors them to a JSONL
// file so a session survives a restart. No external database is required, which
// keeps the binary small and dependency-free.
package store

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/qub-official/myTraffic/internal/httpql"
)

// Flow is a single captured request and its response.
type Flow struct {
	ID         int64             `json:"id"`
	Time       time.Time         `json:"time"`
	Method     string            `json:"method"`
	Scheme     string            `json:"scheme"`
	Host       string            `json:"host"`
	Path       string            `json:"path"`
	ReqHeaders map[string]string `json:"reqHeaders"`
	ReqBody    string            `json:"reqBody"`
	Status     int               `json:"status"`
	ResHeaders map[string]string `json:"resHeaders"`
	ResBody    string            `json:"resBody"`
	Size       int               `json:"size"`
	DurationMs int64             `json:"durationMs"`
	Comment    string            `json:"comment,omitempty"`
	Color      string            `json:"color,omitempty"`   // row highlight
	Note       string            `json:"note,omitempty"`    // free-text annotation
	Starred    bool              `json:"starred,omitempty"` // organizer flag
}

// Store is a capped, thread-safe collection of flows.
type Store struct {
	mu     sync.RWMutex
	flows  []*Flow
	nextID int64
	cap    int
	w      *bufio.Writer
	f      *os.File
	subs   map[chan *Flow]struct{}
}

// New creates a store holding up to capN flows in memory.
func New(capN int) *Store {
	if capN <= 0 {
		capN = 10000
	}
	return &Store{
		nextID: 1,
		cap:    capN,
		subs:   make(map[chan *Flow]struct{}),
	}
}

// Persist mirrors new flows to a JSONL file at path (appended).
func (s *Store) Persist(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.f = f
	s.w = bufio.NewWriter(f)
	s.mu.Unlock()
	return nil
}

// Close flushes and closes any persistence file.
func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w != nil {
		s.w.Flush()
	}
	if s.f != nil {
		s.f.Close()
	}
}

// Add stores a flow, assigns it an ID, and notifies subscribers.
func (s *Store) Add(f *Flow) *Flow {
	s.mu.Lock()
	f.ID = s.nextID
	s.nextID++
	s.flows = append(s.flows, f)
	if len(s.flows) > s.cap {
		s.flows = s.flows[len(s.flows)-s.cap:]
	}
	if s.w != nil {
		if b, err := json.Marshal(f); err == nil {
			s.w.Write(b)
			s.w.WriteByte('\n')
			s.w.Flush()
		}
	}
	subs := make([]chan *Flow, 0, len(s.subs))
	for ch := range s.subs {
		subs = append(subs, ch)
	}
	s.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- f:
		default:
		}
	}
	return f
}

// Get returns a flow by ID.
func (s *Store) Get(id int64) (*Flow, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.flows {
		if f.ID == id {
			return f, true
		}
	}
	return nil, false
}

// List returns flows matching an HTTPQL filter, newest first, capped at limit.
// An invalid query matches nothing (the error is available via ParseFilter).
func (s *Store) List(filter string, limit int) []*Flow {
	q, err := httpql.Parse(filter)
	if err != nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Flow, 0, limit)
	for i := len(s.flows) - 1; i >= 0; i-- {
		f := s.flows[i]
		if q.Match(f) {
			out = append(out, f)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

// ParseFilter validates an HTTPQL query and returns any syntax error.
func (s *Store) ParseFilter(filter string) error {
	_, err := httpql.Parse(filter)
	return err
}

// Annotate sets a flow's color, note and starred flag. Empty color/note leave
// the existing values unless clear is set.
func (s *Store) Annotate(id int64, color, note string, starred bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.flows {
		if f.ID == id {
			f.Color = color
			f.Note = note
			f.Starred = starred
			return true
		}
	}
	return false
}

// Snapshot returns a copy of all flows (oldest first) for export.
func (s *Store) Snapshot() []*Flow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Flow, len(s.flows))
	copy(out, s.flows)
	return out
}

// Load replaces the store's flows with the given set (used by project load).
func (s *Store) Load(flows []*Flow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flows = flows
	var maxID int64
	for _, f := range flows {
		if f.ID > maxID {
			maxID = f.ID
		}
	}
	s.nextID = maxID + 1
}

// Clear removes all flows from memory.
func (s *Store) Clear() {
	s.mu.Lock()
	s.flows = nil
	s.mu.Unlock()
}

// Subscribe returns a channel that receives newly added flows.
func (s *Store) Subscribe() (chan *Flow, func()) {
	ch := make(chan *Flow, 64)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	cancel := func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
		close(ch)
	}
	return ch, cancel
}
