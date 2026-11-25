// Package httpql implements the HTTPQL query language used to filter traffic,
// following Caido's HTTPQL reference: namespaces (req, resp, row), typed fields,
// comparison operators (eq ne gt gte lt lte cont ncont like nlike regex nregex),
// AND/OR logic with parentheses, and a bare quoted string that searches the raw
// request and response together. Whitespace between terms is an implicit AND.
//
// It evaluates against any value implementing Record, so it does not import the
// store package (which would create an import cycle); store.Flow implements it.
package httpql

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Record is the view of a captured flow that a query is evaluated against.
type Record interface {
	// Str returns a string field. ns is "req"/"resp"/"row"; name is the field
	// (host, method, path, ext, query, body, raw, header); key is an optional
	// header name for header["key"].
	Str(ns, name, key string) (string, bool)
	// Num returns a numeric field (resp.code, resp.len, resp.roundtrip, req.len,
	// req.port, row.id).
	Num(ns, name string) (float64, bool)
	// Date returns a date field (req.created_at).
	Date(ns, name string) (time.Time, bool)
	// Raw is the combined raw text searched by a bare string.
	Raw() string
}

// Query is a compiled HTTPQL expression.
type Query struct {
	root node
}

// Match reports whether r satisfies the query. A nil/empty query matches all.
func (q *Query) Match(r Record) bool {
	if q == nil || q.root == nil {
		return true
	}
	return q.root.eval(r)
}

// Parse compiles an HTTPQL string. An empty string yields a match-all query.
func Parse(s string) (*Query, error) {
	toks, err := lex(s)
	if err != nil {
		return nil, err
	}
	if len(toks) == 0 {
		return &Query{}, nil
	}
	p := &parser{toks: toks}
	n, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.toks) {
		return nil, errAt("unexpected trailing input")
	}
	return &Query{root: n}, nil
}

// ---- AST ----

type node interface{ eval(r Record) bool }

type andNode struct{ a, b node }

func (n andNode) eval(r Record) bool { return n.a.eval(r) && n.b.eval(r) }

type orNode struct{ a, b node }

func (n orNode) eval(r Record) bool { return n.a.eval(r) || n.b.eval(r) }

type rawNode struct{ s string }

func (n rawNode) eval(r Record) bool {
	return strings.Contains(strings.ToLower(r.Raw()), strings.ToLower(n.s))
}

type cmpNode struct {
	ns, field, key string
	op             string
	val            string
	re             *regexp.Regexp // compiled for regex/nregex
}

func (n cmpNode) eval(r Record) bool {
	switch n.op {
	case "eq", "ne", "gt", "gte", "lt", "lte":
		// Try numeric first, then date, then string equality for eq/ne.
		if got, ok := r.Num(n.ns, n.field); ok {
			want, err := strconv.ParseFloat(n.val, 64)
			if err != nil {
				return false
			}
			return numCmp(got, want, n.op)
		}
		if got, ok := r.Date(n.ns, n.field); ok {
			if want, err := parseDate(n.val); err == nil {
				return numCmp(float64(got.Unix()), float64(want.Unix()), n.op)
			}
			return false
		}
		got, _ := r.Str(n.ns, n.field, n.key)
		switch n.op {
		case "eq":
			return got == n.val
		case "ne":
			return got != n.val
		default:
			return false
		}
	case "cont", "ncont":
		got := n.getStr(r)
		c := strings.Contains(strings.ToLower(got), strings.ToLower(n.val))
		return c == (n.op == "cont")
	case "like", "nlike":
		got := n.getStr(r)
		m := likeMatch(got, n.val)
		return m == (n.op == "like")
	case "regex", "nregex":
		got := n.getStr(r)
		if n.re == nil {
			return false
		}
		return n.re.MatchString(got) == (n.op == "regex")
	}
	return false
}

// getStr returns the field as a string, coercing numeric and date fields so
// string operators (cont, like, regex) work on resp.code, resp.len, etc.
func (n cmpNode) getStr(r Record) string {
	if s, ok := r.Str(n.ns, n.field, n.key); ok {
		return s
	}
	if v, ok := r.Num(n.ns, n.field); ok {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	if d, ok := r.Date(n.ns, n.field); ok {
		return d.Format(time.RFC3339)
	}
	return ""
}

func numCmp(a, b float64, op string) bool {
	switch op {
	case "eq":
		return a == b
	case "ne":
		return a != b
	case "gt":
		return a > b
	case "gte":
		return a >= b
	case "lt":
		return a < b
	case "lte":
		return a <= b
	}
	return false
}

// likeMatch implements SQLite LIKE: % matches any run, _ matches one char,
// case-insensitive.
func likeMatch(s, pattern string) bool {
	var b strings.Builder
	b.WriteString("(?is)^")
	for _, r := range pattern {
		switch r {
		case '%':
			b.WriteString(".*")
		case '_':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(s)
}

func parseDate(s string) (time.Time, error) {
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02", time.RFC1123, time.RFC822}
	var lastErr error
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, lastErr
}
