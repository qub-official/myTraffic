package proxy

import (
	"regexp"
	"sync"
)

// Rule rewrites part of a request or response. It mirrors Burp's
// "Match and Replace" / Caido's "Tamper" rules.
type Rule struct {
	ID      int64  `json:"id"`
	Enabled bool   `json:"enabled"`
	Name    string `json:"name"`
	// Target: "req.header", "req.body", "resp.header", "resp.body".
	Target string `json:"target"`
	// Match is a regular expression; Replace is the replacement (supports $1).
	Match   string `json:"match"`
	Replace string `json:"replace"`

	re *regexp.Regexp
}

// RuleSet is a thread-safe, ordered collection of rules.
type RuleSet struct {
	mu     sync.RWMutex
	rules  []*Rule
	nextID int64
}

// NewRuleSet creates an empty rule set.
func NewRuleSet() *RuleSet { return &RuleSet{nextID: 1} }

// Add compiles and appends a rule.
func (rs *RuleSet) Add(r *Rule) (*Rule, error) {
	re, err := regexp.Compile(r.Match)
	if err != nil {
		return nil, err
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r.ID = rs.nextID
	rs.nextID++
	r.re = re
	rs.rules = append(rs.rules, r)
	return r, nil
}

// Remove deletes a rule by ID.
func (rs *RuleSet) Remove(id int64) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := rs.rules[:0]
	for _, r := range rs.rules {
		if r.ID != id {
			out = append(out, r)
		}
	}
	rs.rules = out
}

// List returns a snapshot of all rules.
func (rs *RuleSet) List() []*Rule {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	out := make([]*Rule, len(rs.rules))
	copy(out, rs.rules)
	return out
}

// apply runs all enabled rules for the given target over s and returns the
// rewritten string plus whether anything changed.
func (rs *RuleSet) apply(target, s string) (string, bool) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	changed := false
	for _, r := range rs.rules {
		if !r.Enabled || r.Target != target || r.re == nil {
			continue
		}
		ns := r.re.ReplaceAllString(s, r.Replace)
		if ns != s {
			changed = true
			s = ns
		}
	}
	return s, changed
}
