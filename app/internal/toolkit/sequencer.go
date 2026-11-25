package toolkit

import (
	"math"
	"sort"
)

// SeqReport summarizes the randomness of a set of tokens, in the spirit of
// Burp's Sequencer. It is an entropy-based estimate, not a certification.
type SeqReport struct {
	Count          int       `json:"count"`
	Unique         int       `json:"unique"`
	MinLen         int       `json:"minLen"`
	MaxLen         int       `json:"maxLen"`
	CharsetSize    int       `json:"charsetSize"`
	BitsPerChar    float64   `json:"bitsPerChar"`
	BitsPerToken   float64   `json:"bitsPerToken"`   // sum of per-position entropy
	PerPosition    []float64 `json:"perPosition"`    // Shannon entropy per character index
	Verdict        string    `json:"verdict"`
	UniqueFraction float64   `json:"uniqueFraction"`
}

// Sequence analyzes token randomness.
func Sequence(tokens []string) SeqReport {
	var r SeqReport
	r.Count = len(tokens)
	if r.Count == 0 {
		r.Verdict = "no tokens"
		return r
	}

	seen := make(map[string]bool, len(tokens))
	charset := make(map[rune]bool)
	r.MinLen = math.MaxInt32
	maxLen := 0
	for _, t := range tokens {
		seen[t] = true
		if len(t) < r.MinLen {
			r.MinLen = len(t)
		}
		if len(t) > maxLen {
			maxLen = len(t)
		}
		for _, c := range t {
			charset[c] = true
		}
	}
	r.Unique = len(seen)
	r.MaxLen = maxLen
	r.CharsetSize = len(charset)
	r.UniqueFraction = float64(r.Unique) / float64(r.Count)

	// Per-position Shannon entropy.
	r.PerPosition = make([]float64, maxLen)
	total := 0.0
	for pos := 0; pos < maxLen; pos++ {
		counts := make(map[rune]int)
		n := 0
		for _, t := range tokens {
			runes := []rune(t)
			if pos < len(runes) {
				counts[runes[pos]]++
				n++
			}
		}
		h := shannon(counts, n)
		r.PerPosition[pos] = h
		total += h
	}
	r.BitsPerToken = total

	// Overall bits per character (pooled).
	pooled := make(map[rune]int)
	totalChars := 0
	for _, t := range tokens {
		for _, c := range t {
			pooled[c]++
			totalChars++
		}
	}
	r.BitsPerChar = shannon(pooled, totalChars)

	switch {
	case r.Unique < r.Count:
		r.Verdict = "weak — duplicate tokens found"
	case r.BitsPerToken >= 64:
		r.Verdict = "strong — high effective entropy"
	case r.BitsPerToken >= 32:
		r.Verdict = "moderate — usable but not ideal"
	default:
		r.Verdict = "weak — low effective entropy"
	}
	return r
}

func shannon(counts map[rune]int, n int) float64 {
	if n == 0 {
		return 0
	}
	// Deterministic iteration for stable results.
	keys := make([]rune, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	h := 0.0
	for _, k := range keys {
		p := float64(counts[k]) / float64(n)
		h -= p * math.Log2(p)
	}
	return h
}
