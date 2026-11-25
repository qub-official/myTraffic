package store

import (
	"strconv"
	"sync"
	"time"

	"github.com/qub-official/myTraffic/internal/httpql"
)

// WSMessage is one captured WebSocket frame.
type WSMessage struct {
	ID        int64     `json:"id"`
	Time      time.Time `json:"time"`
	Scheme    string    `json:"scheme"` // ws | wss
	Host      string    `json:"host"`
	Path      string    `json:"path"`
	Port      int       `json:"port"`
	Direction string    `json:"direction"` // client | server
	Format    string    `json:"format"`    // Text | Binary | Ping | Pong | Close
	Payload   string    `json:"payload"`
	Len       int       `json:"len"`
}

// WSStore holds captured WebSocket messages with StreamQL filtering.
type WSStore struct {
	mu     sync.RWMutex
	msgs   []*WSMessage
	nextID int64
	cap    int
	subs   map[chan *WSMessage]struct{}
}

// NewWSStore creates a WebSocket message store capped at capN.
func NewWSStore(capN int) *WSStore {
	if capN <= 0 {
		capN = 20000
	}
	return &WSStore{nextID: 1, cap: capN, subs: make(map[chan *WSMessage]struct{})}
}

// Add stores a message and notifies subscribers.
func (s *WSStore) Add(m *WSMessage) *WSMessage {
	s.mu.Lock()
	m.ID = s.nextID
	s.nextID++
	m.Len = len(m.Payload)
	s.msgs = append(s.msgs, m)
	if len(s.msgs) > s.cap {
		s.msgs = s.msgs[len(s.msgs)-s.cap:]
	}
	subs := make([]chan *WSMessage, 0, len(s.subs))
	for ch := range s.subs {
		subs = append(subs, ch)
	}
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- m:
		default:
		}
	}
	return m
}

// List returns messages matching a StreamQL filter, newest first.
func (s *WSStore) List(filter string, limit int) []*WSMessage {
	q, err := httpql.Parse(filter)
	if err != nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*WSMessage, 0, limit)
	for i := len(s.msgs) - 1; i >= 0; i-- {
		if q.Match(s.msgs[i]) {
			out = append(out, s.msgs[i])
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out
}

// Clear removes all messages.
func (s *WSStore) Clear() {
	s.mu.Lock()
	s.msgs = nil
	s.mu.Unlock()
}

// Subscribe returns a channel of new messages and a cancel func.
func (s *WSStore) Subscribe() (chan *WSMessage, func()) {
	ch := make(chan *WSMessage, 64)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
		close(ch)
	}
}

// --- httpql.Record implementation (StreamQL: ws.* and stream.* namespaces) ---

func (m *WSMessage) Str(ns, name, key string) (string, bool) {
	switch ns {
	case "ws":
		switch name {
		case "direction":
			return m.Direction, true
		case "format":
			return m.Format, true
		case "raw", "body":
			return m.Payload, true
		}
	case "stream":
		switch name {
		case "host":
			return m.Host, true
		case "path":
			return m.Path, true
		case "protocol":
			return m.Scheme, true
		case "tls":
			if m.Scheme == "wss" {
				return "true", true
			}
			return "false", true
		}
	}
	return "", false
}

func (m *WSMessage) Num(ns, name string) (float64, bool) {
	if ns == "ws" && name == "len" {
		return float64(m.Len), true
	}
	if ns == "stream" && name == "port" {
		return float64(m.Port), true
	}
	return 0, false
}

func (m *WSMessage) Date(ns, name string) (time.Time, bool) {
	if ns == "ws" && name == "created_at" {
		return m.Time, true
	}
	return time.Time{}, false
}

func (m *WSMessage) Raw() string { return m.Direction + " " + m.Format + " " + m.Payload + " " + strconv.Itoa(m.Port) }
