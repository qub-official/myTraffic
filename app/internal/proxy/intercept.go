package proxy

import (
	"sync"
	"sync/atomic"
)

// Intercepted is a request paused for user review.
type Intercepted struct {
	ID     int64             `json:"id"`
	Method string            `json:"method"`
	URL    string            `json:"url"`
	Header map[string]string `json:"header"`
	Body   string            `json:"body"`

	resume chan decision
}

type decision struct {
	action string // "forward" or "drop"
	method string
	url    string
	header map[string]string
	body   string
}

// Interceptor holds requests paused awaiting a user decision.
type Interceptor struct {
	enabled atomic.Bool
	mu      sync.Mutex
	pending map[int64]*Intercepted
	nextID  int64
}

// NewInterceptor creates a disabled interceptor.
func NewInterceptor() *Interceptor {
	return &Interceptor{pending: make(map[int64]*Intercepted)}
}

// SetEnabled turns interception on or off. When turned off, all pending
// requests are released unchanged so nothing hangs.
func (ic *Interceptor) SetEnabled(on bool) {
	ic.enabled.Store(on)
	if !on {
		ic.mu.Lock()
		for _, p := range ic.pending {
			select {
			case p.resume <- decision{action: "forward"}:
			default:
			}
		}
		ic.mu.Unlock()
	}
}

// Enabled reports whether interception is on.
func (ic *Interceptor) Enabled() bool { return ic.enabled.Load() }

// hold registers a paused request and blocks until a decision arrives.
func (ic *Interceptor) hold(method, url string, header map[string]string, body string) decision {
	ic.mu.Lock()
	ic.nextID++
	id := ic.nextID
	p := &Intercepted{
		ID:     id,
		Method: method,
		URL:    url,
		Header: header,
		Body:   body,
		resume: make(chan decision, 1),
	}
	ic.pending[id] = p
	ic.mu.Unlock()

	d := <-p.resume

	ic.mu.Lock()
	delete(ic.pending, id)
	ic.mu.Unlock()
	return d
}

// Queue returns the currently paused requests.
func (ic *Interceptor) Queue() []*Intercepted {
	ic.mu.Lock()
	defer ic.mu.Unlock()
	out := make([]*Intercepted, 0, len(ic.pending))
	for _, p := range ic.pending {
		out = append(out, p)
	}
	return out
}

// ResolveExported applies a decision from the API/CLI layer to a paused
// request. action is "forward" or "drop"; the other fields optionally override
// the outgoing request when forwarding.
func (ic *Interceptor) ResolveExported(id int64, action, method, url string, header map[string]string, body string) bool {
	if action == "" {
		action = "forward"
	}
	return ic.Resolve(id, decision{
		action: action,
		method: method,
		url:    url,
		header: header,
		body:   body,
	})
}

// Resolve applies a decision to a paused request. Returns false if unknown.
func (ic *Interceptor) Resolve(id int64, d decision) bool {
	ic.mu.Lock()
	p, ok := ic.pending[id]
	ic.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case p.resume <- d:
		return true
	default:
		return false
	}
}
