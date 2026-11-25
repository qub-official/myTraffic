package httpql

import (
	"testing"
	"time"
)

// rec is a test Record.
type rec struct {
	str  map[string]string
	num  map[string]float64
	raw  string
}

func (r rec) Str(ns, name, key string) (string, bool) {
	k := ns + "." + name
	if key != "" {
		k += "[" + key + "]"
	}
	v, ok := r.str[k]
	return v, ok
}
func (r rec) Num(ns, name string) (float64, bool) {
	v, ok := r.num[ns+"."+name]
	return v, ok
}
func (r rec) Date(ns, name string) (time.Time, bool) { return time.Time{}, false }
func (r rec) Raw() string                            { return r.raw }

func mustMatch(t *testing.T, q string, r Record, want bool) {
	t.Helper()
	query, err := Parse(q)
	if err != nil {
		t.Fatalf("parse %q: %v", q, err)
	}
	if got := query.Match(r); got != want {
		t.Fatalf("query %q matched=%v want %v", q, got, want)
	}
}

func TestHTTPQL(t *testing.T) {
	r := rec{
		str: map[string]string{
			"req.host":                  "api.example.com",
			"req.method":                "GET",
			"req.path":                  "/v1/users",
			"resp.header[Content-Type]": "application/json",
		},
		num: map[string]float64{"resp.code": 403, "resp.len": 1200},
		raw: "GET /v1/users secret-token",
	}
	mustMatch(t, `req.host.cont:"example"`, r, true)
	mustMatch(t, `req.path.eq:"/v1/users"`, r, true)
	mustMatch(t, `resp.header["Content-Type"].cont:"json"`, r, true)
	mustMatch(t, `resp.len.gte:1000`, r, true)
}

func TestOperators(t *testing.T) {
	r := rec{
		str: map[string]string{"req.host": "api.example.com", "req.method": "GET"},
		num: map[string]float64{"resp.code": 403},
		raw: "hello secret world",
	}
	mustMatch(t, `req.host.cont:"example"`, r, true)
	mustMatch(t, `req.host.cont:"nope"`, r, false)
	mustMatch(t, `req.host.ncont:"nope"`, r, true)
	mustMatch(t, `req.method.eq:"GET"`, r, true)
	mustMatch(t, `req.method.ne:"POST"`, r, true)
	mustMatch(t, `resp.code.eq:403`, r, true)
	mustMatch(t, `resp.code.gte:400`, r, true)
	mustMatch(t, `resp.code.lt:400`, r, false)
	mustMatch(t, `resp.code.regex:"4\\d\\d"`, r, true)
	mustMatch(t, `req.host.like:"api.%.com"`, r, true)
	// logic
	mustMatch(t, `resp.code.gte:400 AND req.method.eq:"GET"`, r, true)
	mustMatch(t, `resp.code.lt:400 OR req.method.eq:"GET"`, r, true)
	mustMatch(t, `resp.code.lt:400 AND req.method.eq:"GET"`, r, false)
	// implicit AND (whitespace)
	mustMatch(t, `resp.code.gte:400 req.method.eq:"GET"`, r, true)
	// parentheses
	mustMatch(t, `(resp.code.lt:400 OR req.method.eq:"GET") AND req.host.cont:"example"`, r, true)
	// bare string -> raw search
	mustMatch(t, `secret`, r, true)
	mustMatch(t, `"missing"`, r, false)
}
