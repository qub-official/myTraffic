package store

import (
	"strconv"
	"strings"
	"time"
)

// Str implements httpql.Record.
func (f *Flow) Str(ns, name, key string) (string, bool) {
	switch ns {
	case "req":
		switch name {
		case "host":
			return f.Host, true
		case "method":
			return f.Method, true
		case "path":
			return f.Path, true
		case "ext":
			return pathExt(f.Path), true
		case "query":
			return pathQuery(f.Path), true
		case "body":
			return f.ReqBody, true
		case "raw":
			return f.reqRaw(), true
		case "tls":
			if f.Scheme == "https" {
				return "true", true
			}
			return "false", true
		case "header":
			return headerLookup(f.ReqHeaders, key), true
		}
	case "resp":
		switch name {
		case "body":
			return f.ResBody, true
		case "raw":
			return f.respRaw(), true
		case "header":
			return headerLookup(f.ResHeaders, key), true
		case "code":
			return strconv.Itoa(f.Status), true
		}
	case "row":
		if name == "id" {
			return strconv.FormatInt(f.ID, 10), true
		}
	}
	return "", false
}

// Num implements httpql.Record.
func (f *Flow) Num(ns, name string) (float64, bool) {
	switch ns {
	case "req":
		switch name {
		case "len":
			return float64(len(f.ReqBody)), true
		}
	case "resp":
		switch name {
		case "code":
			return float64(f.Status), true
		case "len":
			return float64(f.Size), true
		case "roundtrip":
			return float64(f.DurationMs), true
		}
	case "row":
		if name == "id" {
			return float64(f.ID), true
		}
	}
	return 0, false
}

// Date implements httpql.Record.
func (f *Flow) Date(ns, name string) (time.Time, bool) {
	if ns == "req" && name == "created_at" {
		return f.Time, true
	}
	return time.Time{}, false
}

// Raw implements httpql.Record (bare-string search target).
func (f *Flow) Raw() string { return f.reqRaw() + "\n" + f.respRaw() }

func (f *Flow) reqRaw() string {
	var b strings.Builder
	b.WriteString(f.Method)
	b.WriteByte(' ')
	b.WriteString(f.Path)
	b.WriteByte('\n')
	for k, v := range f.ReqHeaders {
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(f.ReqBody)
	return b.String()
}

func (f *Flow) respRaw() string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(f.Status))
	b.WriteByte('\n')
	for k, v := range f.ResHeaders {
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(f.ResBody)
	return b.String()
}

func headerLookup(h map[string]string, key string) string {
	if key == "" {
		// All headers joined.
		parts := make([]string, 0, len(h))
		for k, v := range h {
			parts = append(parts, k+": "+v)
		}
		return strings.Join(parts, "\n")
	}
	if v, ok := h[key]; ok {
		return v
	}
	// Case-insensitive fallback.
	for k, v := range h {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func pathExt(p string) string {
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		p = p[i+1:]
	}
	if i := strings.LastIndexByte(p, '.'); i >= 0 {
		return p[i+1:]
	}
	return ""
}

func pathQuery(p string) string {
	if i := strings.IndexByte(p, '?'); i >= 0 {
		return p[i+1:]
	}
	return ""
}
