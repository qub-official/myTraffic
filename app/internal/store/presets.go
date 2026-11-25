package store

import "sync"

// Preset is a saved, named HTTPQL query (a filter preset).
type Preset struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Query string `json:"query"`
}

// Presets is a thread-safe list of saved queries.
type Presets struct {
	mu     sync.RWMutex
	items  []*Preset
	nextID int64
}

// NewPresets creates an empty preset list.
func NewPresets() *Presets { return &Presets{nextID: 1} }

// Add stores a preset.
func (p *Presets) Add(x *Preset) *Preset {
	p.mu.Lock()
	defer p.mu.Unlock()
	x.ID = p.nextID
	p.nextID++
	p.items = append(p.items, x)
	return x
}

// Remove deletes a preset by ID.
func (p *Presets) Remove(id int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.items[:0]
	for _, x := range p.items {
		if x.ID != id {
			out = append(out, x)
		}
	}
	p.items = out
}

// List returns all presets.
func (p *Presets) List() []*Preset {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*Preset, len(p.items))
	copy(out, p.items)
	return out
}

// Load replaces the presets (used by project load).
func (p *Presets) Load(items []*Preset) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items = items
	var maxID int64
	for _, x := range items {
		if x.ID > maxID {
			maxID = x.ID
		}
	}
	p.nextID = maxID + 1
}
