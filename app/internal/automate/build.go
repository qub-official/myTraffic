package automate

import (
	"fmt"
	"strconv"
	"strings"
)

// template is a string split around marker-delimited insertion points. The
// rendered string is segs[0] + val[0] + segs[1] + val[1] + ... + segs[n].
type template struct {
	segs  []string // len == holes+1
	bases []string // len == holes; original text between markers
}

func (t template) holes() int { return len(t.bases) }

func (t template) render(vals []string) string {
	if len(t.bases) == 0 {
		return t.segs[0]
	}
	var b strings.Builder
	for i, seg := range t.segs {
		b.WriteString(seg)
		if i < len(vals) {
			b.WriteString(vals[i])
		}
	}
	return b.String()
}

// parseTemplate splits s on pairs of marker. Text between the first and second
// marker is one insertion point (its content is the base value), and so on.
func parseTemplate(s, marker string) template {
	if marker == "" || !strings.Contains(s, marker) {
		return template{segs: []string{s}}
	}
	parts := strings.Split(s, marker)
	// parts alternate: literal, base, literal, base, ... An even number of
	// markers yields segs = literals at even indices, bases at odd indices.
	var t template
	for i := 0; i < len(parts); i++ {
		if i%2 == 0 {
			t.segs = append(t.segs, parts[i])
		} else {
			t.bases = append(t.bases, parts[i])
		}
	}
	// If markers were unbalanced, the trailing base has no closing segment;
	// fold it back so render stays consistent.
	if len(t.segs) == len(t.bases) {
		last := t.bases[len(t.bases)-1]
		t.bases = t.bases[:len(t.bases)-1]
		t.segs[len(t.segs)-1] += marker + last
	}
	return t
}

// expand turns a PayloadSet into a concrete list of payloads.
func expand(ps PayloadSet) ([]string, error) {
	switch ps.Type {
	case "", "list":
		return capList(ps.List), nil
	case "null":
		c := ps.Count
		if c <= 0 {
			c = 1
		}
		if c > maxPerSet {
			c = maxPerSet
		}
		out := make([]string, c)
		for i := range out {
			out[i] = ""
		}
		return out, nil
	case "numbers":
		return numbers(ps)
	case "brute":
		return brute(ps)
	default:
		return nil, fmt.Errorf("unknown payload type %q", ps.Type)
	}
}

func capList(l []string) []string {
	if len(l) > maxPerSet {
		return l[:maxPerSet]
	}
	return l
}

func numbers(ps PayloadSet) ([]string, error) {
	step := ps.Step
	if step == 0 {
		step = 1
	}
	if (ps.To > ps.From && step < 0) || (ps.To < ps.From && step > 0) {
		return nil, fmt.Errorf("numbers: step direction does not reach 'to'")
	}
	var out []string
	count := 0
	if step > 0 {
		for v := ps.From; v <= ps.To; v += step {
			out = append(out, formatNum(v))
			if count++; count > maxPerSet {
				break
			}
		}
	} else {
		for v := ps.From; v >= ps.To; v += step {
			out = append(out, formatNum(v))
			if count++; count > maxPerSet {
				break
			}
		}
	}
	return out, nil
}

func formatNum(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func brute(ps PayloadSet) ([]string, error) {
	cs := ps.Charset
	if cs == "" {
		cs = "abcdefghijklmnopqrstuvwxyz0123456789"
	}
	min, max := ps.Min, ps.Max
	if min <= 0 {
		min = 1
	}
	if max < min {
		max = min
	}
	var out []string
	// Generate length-by-length so output is ordered by increasing length.
	for l := min; l <= max; l++ {
		genFixed(cs, l, "", &out)
		if len(out) > maxPerSet {
			break
		}
	}
	return out, nil
}

func genFixed(cs string, l int, prefix string, out *[]string) {
	if len(*out) > maxPerSet {
		return
	}
	if l == 0 {
		*out = append(*out, prefix)
		return
	}
	for _, c := range cs {
		genFixed(cs, l-1, prefix+string(c), out)
		if len(*out) > maxPerSet {
			return
		}
	}
}

// buildVectors produces the sequence of payload vectors (each length n) for the
// chosen attack type. base holds the original position values.
func buildVectors(attack string, n int, base []string, sets [][]string) ([][]string, error) {
	switch strings.ToLower(attack) {
	case "", "sniper":
		return sniper(n, base, sets)
	case "batteringram", "battering_ram", "ram":
		return batteringRam(n, sets)
	case "pitchfork":
		return pitchfork(n, base, sets)
	case "clusterbomb", "cluster_bomb", "cluster":
		return clusterBomb(n, base, sets)
	default:
		return nil, fmt.Errorf("unknown attack type %q", attack)
	}
}

func firstSet(sets [][]string) ([]string, error) {
	if len(sets) == 0 || len(sets[0]) == 0 {
		return nil, fmt.Errorf("this attack type needs one payload set")
	}
	return sets[0], nil
}

// Sniper: one set, cycled through each position in turn; other positions keep
// their base value.
func sniper(n int, base []string, sets [][]string) ([][]string, error) {
	set, err := firstSet(sets)
	if err != nil {
		return nil, err
	}
	var out [][]string
	for pos := 0; pos < n; pos++ {
		for _, v := range set {
			vec := make([]string, n)
			copy(vec, base)
			vec[pos] = v
			out = append(out, vec)
		}
	}
	return out, nil
}

// Battering ram: one set, same payload in every position at once.
func batteringRam(n int, sets [][]string) ([][]string, error) {
	set, err := firstSet(sets)
	if err != nil {
		return nil, err
	}
	out := make([][]string, 0, len(set))
	for _, v := range set {
		vec := make([]string, n)
		for i := range vec {
			vec[i] = v
		}
		out = append(out, vec)
	}
	return out, nil
}

// Pitchfork: one set per position, advanced in lockstep (min length).
func pitchfork(n int, base []string, sets [][]string) ([][]string, error) {
	if len(sets) < n {
		return nil, fmt.Errorf("pitchfork needs one payload set per position (%d)", n)
	}
	minLen := -1
	for i := 0; i < n; i++ {
		if minLen < 0 || len(sets[i]) < minLen {
			minLen = len(sets[i])
		}
	}
	if minLen <= 0 {
		return nil, fmt.Errorf("pitchfork: a payload set is empty")
	}
	var out [][]string
	for i := 0; i < minLen; i++ {
		vec := make([]string, n)
		copy(vec, base)
		for pos := 0; pos < n; pos++ {
			vec[pos] = sets[pos][i]
		}
		out = append(out, vec)
	}
	return out, nil
}

// Cluster bomb: one set per position, every combination.
func clusterBomb(n int, base []string, sets [][]string) ([][]string, error) {
	if len(sets) < n {
		return nil, fmt.Errorf("cluster bomb needs one payload set per position (%d)", n)
	}
	total := 1
	for pos := 0; pos < n; pos++ {
		if len(sets[pos]) == 0 {
			return nil, fmt.Errorf("cluster bomb: payload set %d is empty", pos+1)
		}
		total *= len(sets[pos])
		if total > maxJobs {
			return nil, fmt.Errorf("cluster bomb combinations exceed the %d cap", maxJobs)
		}
	}
	var out [][]string
	idx := make([]int, n)
	for {
		vec := make([]string, n)
		copy(vec, base)
		for pos := 0; pos < n; pos++ {
			vec[pos] = sets[pos][idx[pos]]
		}
		out = append(out, vec)
		// increment odometer
		k := n - 1
		for k >= 0 {
			idx[k]++
			if idx[k] < len(sets[k]) {
				break
			}
			idx[k] = 0
			k--
		}
		if k < 0 {
			break
		}
	}
	return out, nil
}
