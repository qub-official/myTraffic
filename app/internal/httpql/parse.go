package httpql

import (
	"fmt"
	"regexp"
	"strings"
)

type tokKind int

const (
	tClause   tokKind = iota // field.op:value
	tString                  // standalone quoted string -> raw search
	tBareWord                // and / or / bare raw term
	tLParen
	tRParen
)

type token struct {
	kind tokKind
	key  string // for tClause: the dotted path including op
	val  string // for tClause: the value; for tString/tBareWord: the text
}

type parseError struct{ msg string }

func (e parseError) Error() string { return "httpql: " + e.msg }
func errAt(m string) error         { return parseError{msg: m} }

var operators = map[string]bool{
	"eq": true, "ne": true, "gt": true, "gte": true, "lt": true, "lte": true,
	"cont": true, "ncont": true, "like": true, "nlike": true,
	"regex": true, "nregex": true,
}

// lex tokenizes an HTTPQL string in the Caido syntax: clauses look like
// `req.host.cont:"example"` or `resp.code.gte:400`; `AND`/`OR` join them;
// parentheses group; a standalone quoted string is a raw search.
func lex(s string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			toks = append(toks, token{kind: tLParen})
			i++
		case c == ')':
			toks = append(toks, token{kind: tRParen})
			i++
		case c == '"':
			val, ni, err := readQuoted(s, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, token{kind: tString, val: val})
			i = ni
		default:
			// Read a KEY until ':' or whitespace/paren, honoring [..] brackets.
			var key strings.Builder
			j := i
			for j < len(s) {
				ch := s[j]
				if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '(' || ch == ')' || ch == ':' {
					break
				}
				if ch == '[' {
					for j < len(s) && s[j] != ']' {
						key.WriteByte(s[j])
						j++
					}
					if j < len(s) {
						key.WriteByte(']')
						j++
					}
					continue
				}
				key.WriteByte(ch)
				j++
			}
			if j < len(s) && s[j] == ':' {
				// It's a clause; read the value.
				j++ // skip ':'
				var val string
				if j < len(s) && s[j] == '"' {
					v, ni, err := readQuoted(s, j)
					if err != nil {
						return nil, err
					}
					val, j = v, ni
				} else {
					var vb strings.Builder
					for j < len(s) {
						ch := s[j]
						if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '(' || ch == ')' {
							break
						}
						vb.WriteByte(ch)
						j++
					}
					val = vb.String()
				}
				toks = append(toks, token{kind: tClause, key: key.String(), val: val})
				i = j
			} else {
				toks = append(toks, token{kind: tBareWord, val: key.String()})
				i = j
			}
		}
	}
	return toks, nil
}

func readQuoted(s string, start int) (string, int, error) {
	j := start + 1
	var b strings.Builder
	for j < len(s) && s[j] != '"' {
		if s[j] == '\\' && j+1 < len(s) {
			j++
		}
		b.WriteByte(s[j])
		j++
	}
	if j >= len(s) {
		return "", 0, errAt("unterminated string")
	}
	return b.String(), j + 1, nil
}

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() (token, bool) {
	if p.pos < len(p.toks) {
		return p.toks[p.pos], true
	}
	return token{}, false
}

func (p *parser) next() (token, bool) {
	t, ok := p.peek()
	if ok {
		p.pos++
	}
	return t, ok
}

func (p *parser) parseOr() (node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		t, ok := p.peek()
		if !ok || t.kind != tBareWord || !strings.EqualFold(t.val, "or") {
			break
		}
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = orNode{left, right}
	}
	return left, nil
}

func (p *parser) parseAnd() (node, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for {
		t, ok := p.peek()
		if !ok || t.kind == tRParen {
			break
		}
		if t.kind == tBareWord && strings.EqualFold(t.val, "or") {
			break
		}
		// Explicit AND is optional; whitespace between terms implies AND.
		if t.kind == tBareWord && strings.EqualFold(t.val, "and") {
			p.next()
		}
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = andNode{left, right}
	}
	return left, nil
}

func (p *parser) parseTerm() (node, error) {
	t, ok := p.next()
	if !ok {
		return nil, errAt("unexpected end of query")
	}
	switch t.kind {
	case tLParen:
		n, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		end, ok := p.next()
		if !ok || end.kind != tRParen {
			return nil, errAt("expected closing parenthesis")
		}
		return n, nil
	case tString:
		return rawNode{s: t.val}, nil
	case tBareWord:
		if strings.EqualFold(t.val, "and") || strings.EqualFold(t.val, "or") {
			return nil, errAt("unexpected '" + t.val + "'")
		}
		return rawNode{s: t.val}, nil
	case tClause:
		return p.buildClause(t.key, t.val)
	}
	return nil, errAt("unexpected token")
}

// buildClause splits a dotted path like req.host.cont or
// req.header["Content-Type"].gte into field + operator.
func (p *parser) buildClause(key, val string) (node, error) {
	lastDot := strings.LastIndexByte(key, '.')
	if lastDot < 0 {
		return nil, errAt("clause missing operator: " + key)
	}
	op := strings.ToLower(key[lastDot+1:])
	fieldPath := key[:lastDot]
	if !operators[op] {
		return nil, errAt("unknown operator: " + op)
	}
	ns, name, hkey := splitField(fieldPath)
	n := cmpNode{ns: ns, field: name, key: hkey, op: op, val: val}
	if op == "regex" || op == "nregex" {
		re, err := regexp.Compile(val)
		if err != nil {
			return nil, errAt(fmt.Sprintf("invalid regex %q: %v", val, err))
		}
		n.re = re
	}
	return n, nil
}

// splitField parses req.header["X"] into ns=req, name=header, key=X.
func splitField(f string) (ns, name, key string) {
	if i := strings.IndexByte(f, '['); i >= 0 {
		inner := strings.TrimSuffix(f[i+1:], "]")
		key = strings.Trim(inner, `"`)
		f = f[:i]
	}
	parts := strings.SplitN(f, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1], key
	}
	return "", parts[0], key
}
