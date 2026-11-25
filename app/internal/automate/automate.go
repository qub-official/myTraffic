// Package automate implements the fuzzing engine behind myTraffic's Automate
// tab — the equivalent of Burp Intruder / Caido Automate. It supports the four
// classic attack types (Sniper, Battering ram, Pitchfork, Cluster bomb),
// several payload generators, response grep-match and grep-extract, and a
// bounded concurrent request engine. It is transport-agnostic so it reuses the
// proxy's HTTP transport.
package automate

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxPerSet  = 200000 // cap on a single payload set
	maxJobs    = 500000 // cap on total requests in one attack
	maxBodyCap = 2 << 20
)

// PayloadSet describes how to generate payloads for one position.
type PayloadSet struct {
	Type    string   `json:"type"` // list | numbers | null | brute
	List    []string `json:"list,omitempty"`
	From    float64  `json:"from,omitempty"`
	To      float64  `json:"to,omitempty"`
	Step    float64  `json:"step,omitempty"`
	Count   int      `json:"count,omitempty"`   // null payloads
	Charset string   `json:"charset,omitempty"` // brute force
	Min     int      `json:"min,omitempty"`
	Max     int      `json:"max,omitempty"`
}

// Spec defines an attack.
type Spec struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	Body        string            `json:"body"`
	Marker      string            `json:"marker"`     // default "§"
	AttackType  string            `json:"attackType"` // sniper|batteringram|pitchfork|clusterbomb
	Payloads    []PayloadSet      `json:"payloads"`
	Grep        []string          `json:"grep"`
	Extract     []string          `json:"extract"` // regexes
	Concurrency int               `json:"concurrency"`
}

// Result is the outcome of one request in the attack.
type Result struct {
	Index      int             `json:"index"`
	Payloads   []string        `json:"payloads"`
	Status     int             `json:"status"`
	Length     int             `json:"length"`
	DurationMs int64           `json:"durationMs"`
	Grep       map[string]bool `json:"grep,omitempty"`
	Extract    []string        `json:"extract,omitempty"`
	Error      string          `json:"error,omitempty"`
}

// Report is the full attack result set.
type Report struct {
	Count      int      `json:"count"`
	Positions  int      `json:"positions"`
	AttackType string   `json:"attackType"`
	Results    []Result `json:"results"`
}

// Run executes the attack against rt and returns all results ordered by index.
func Run(rt http.RoundTripper, spec Spec) (*Report, error) {
	marker := spec.Marker
	if marker == "" {
		marker = "§"
	}
	// Build templates and count positions.
	urlT := parseTemplate(spec.URL, marker)
	bodyT := parseTemplate(spec.Body, marker)
	headerKeys := make([]string, 0, len(spec.Headers))
	for k := range spec.Headers {
		headerKeys = append(headerKeys, k)
	}
	sort.Strings(headerKeys)
	headerTs := make([]template, len(headerKeys))
	for i, k := range headerKeys {
		headerTs[i] = parseTemplate(spec.Headers[k], marker)
	}

	n := urlT.holes() + bodyT.holes()
	for _, t := range headerTs {
		n += t.holes()
	}
	if n == 0 {
		return nil, fmt.Errorf("no payload positions found; wrap each with %s markers", marker)
	}

	base := make([]string, 0, n)
	base = append(base, urlT.bases...)
	for _, t := range headerTs {
		base = append(base, t.bases...)
	}
	base = append(base, bodyT.bases...)

	// Expand payload sets.
	sets := make([][]string, len(spec.Payloads))
	for i, ps := range spec.Payloads {
		exp, err := expand(ps)
		if err != nil {
			return nil, fmt.Errorf("payload set %d: %w", i+1, err)
		}
		sets[i] = exp
	}

	vectors, err := buildVectors(spec.AttackType, n, base, sets)
	if err != nil {
		return nil, err
	}
	if len(vectors) > maxJobs {
		return nil, fmt.Errorf("attack would send %d requests, over the %d cap", len(vectors), maxJobs)
	}

	greps := spec.Grep
	extracts := make([]*regexp.Regexp, 0, len(spec.Extract))
	for _, e := range spec.Extract {
		re, err := regexp.Compile(e)
		if err != nil {
			return nil, fmt.Errorf("extract regex %q: %w", e, err)
		}
		extracts = append(extracts, re)
	}

	conc := spec.Concurrency
	if conc <= 0 {
		conc = 10
	}
	if conc > 100 {
		conc = 100
	}

	results := make([]Result, len(vectors))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < conc; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				results[idx] = send(rt, spec, urlT, headerKeys, headerTs, bodyT, vectors[idx], idx, greps, extracts)
			}
		}()
	}
	for i := range vectors {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	return &Report{
		Count:      len(results),
		Positions:  n,
		AttackType: spec.AttackType,
		Results:    results,
	}, nil
}

func send(rt http.RoundTripper, spec Spec, urlT template, hkeys []string, hTs []template, bodyT template, vec []string, idx int, greps []string, extracts []*regexp.Regexp) Result {
	// Split the vector back across url / headers / body in the same order.
	p := 0
	url := urlT.render(vec[p : p+urlT.holes()])
	p += urlT.holes()
	headers := make(map[string]string, len(hkeys))
	for i, k := range hkeys {
		h := hTs[i].holes()
		headers[k] = hTs[i].render(vec[p : p+h])
		p += h
	}
	body := bodyT.render(vec[p : p+bodyT.holes()])

	payloads := make([]string, len(vec))
	copy(payloads, vec)
	res := Result{Index: idx, Payloads: payloads}
	method := spec.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		res.Error = err.Error()
		return res
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := rt.RoundTrip(req)
	if err != nil {
		res.Error = err.Error()
		res.DurationMs = time.Since(start).Milliseconds()
		return res
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyCap))
	res.Status = resp.StatusCode
	res.Length = len(b)
	res.DurationMs = time.Since(start).Milliseconds()
	text := string(b)
	if len(greps) > 0 {
		res.Grep = make(map[string]bool, len(greps))
		for _, g := range greps {
			res.Grep[g] = strings.Contains(text, g)
		}
	}
	for _, re := range extracts {
		if m := re.FindStringSubmatch(text); m != nil {
			if len(m) > 1 {
				res.Extract = append(res.Extract, m[1])
			} else {
				res.Extract = append(res.Extract, m[0])
			}
		} else {
			res.Extract = append(res.Extract, "")
		}
	}
	return res
}
