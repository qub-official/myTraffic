// Package api exposes the local control plane: a JSON API plus a server-sent
// events stream that the embedded web UI uses. It binds to localhost only.
package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/qub-official/myTraffic/internal/automate"
	"github.com/qub-official/myTraffic/internal/proxy"
	"github.com/qub-official/myTraffic/internal/store"
	"github.com/qub-official/myTraffic/internal/toolkit"
)

// Server holds the dependencies for the control API.
type Server struct {
	Proxy    *proxy.Proxy
	Store    *store.Store
	Findings *store.Findings
	Presets  *store.Presets
	Static   fs.FS
}

// Handler builds the HTTP handler for the UI + API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/flows", s.flows)
	mux.HandleFunc("/api/flows/", s.flowByID)
	mux.HandleFunc("/api/stream", s.stream)
	mux.HandleFunc("/api/replay", s.replay)
	mux.HandleFunc("/api/rules", s.rules)
	mux.HandleFunc("/api/rules/", s.ruleByID)
	mux.HandleFunc("/api/intercept", s.intercept)
	mux.HandleFunc("/api/intercept/queue", s.interceptQueue)
	mux.HandleFunc("/api/intercept/resolve", s.interceptResolve)
	mux.HandleFunc("/api/scope", s.scope)
	mux.HandleFunc("/api/decode", s.decode)
	mux.HandleFunc("/api/automate", s.automate)
	mux.HandleFunc("/api/sitemap", s.sitemap)
	mux.HandleFunc("/api/compare", s.compare)
	mux.HandleFunc("/api/sequencer", s.sequencer)
	mux.HandleFunc("/api/ws", s.wsList)
	mux.HandleFunc("/api/ws/stream", s.wsStream)
	mux.HandleFunc("/api/upstream", s.upstream)
	mux.HandleFunc("/api/annotate", s.annotate)
	mux.HandleFunc("/api/findings", s.findings)
	mux.HandleFunc("/api/findings/", s.findingByID)
	mux.HandleFunc("/api/presets", s.presets)
	mux.HandleFunc("/api/presets/", s.presetByID)
	mux.HandleFunc("/api/project/export", s.projectExport)
	mux.HandleFunc("/api/project/import", s.projectImport)
	mux.HandleFunc("/api/ca", s.caCert)

	// Static UI.
	if s.Static != nil {
		mux.Handle("/", http.FileServer(http.FS(s.Static)))
	}
	return mux
}

func (s *Server) flows(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		filter := r.URL.Query().Get("filter")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit == 0 {
			limit = 500
		}
		writeJSON(w, s.Store.List(filter, limit))
	case http.MethodDelete:
		s.Store.Clear()
		writeJSON(w, map[string]string{"status": "cleared"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) flowByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/flows/"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	f, ok := s.Store.Get(id)
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, f)
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancel := s.Store.Subscribe()
	defer cancel()

	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case f := <-ch:
			b, _ := json.Marshal(f)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

func (s *Server) replay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Either {"id": n} to replay a captured flow, or a full ReplayReq.
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if idRaw, ok := raw["id"]; ok {
		var id int64
		if json.Unmarshal(idRaw, &id) == nil && id > 0 {
			resp, err := s.Proxy.ReplayFlow(id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			writeJSON(w, resp)
			return
		}
	}
	var rr proxy.ReplayReq
	b, _ := json.Marshal(raw)
	json.Unmarshal(b, &rr)
	resp, err := s.Proxy.Replay(rr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, resp)
}

func (s *Server) rules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.Proxy.Rules.List())
	case http.MethodPost:
		var rule proxy.Rule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		added, err := s.Proxy.Rules.Add(&rule)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, added)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) ruleByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/rules/"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	s.Proxy.Rules.Remove(id)
	writeJSON(w, map[string]string{"status": "removed"})
}

func (s *Server) intercept(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]bool{"enabled": s.Proxy.Inter.Enabled()})
	case http.MethodPost:
		var body struct {
			Enabled bool `json:"enabled"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		s.Proxy.Inter.SetEnabled(body.Enabled)
		writeJSON(w, map[string]bool{"enabled": s.Proxy.Inter.Enabled()})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) interceptQueue(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.Proxy.Inter.Queue())
}

func (s *Server) interceptResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID     int64             `json:"id"`
		Action string            `json:"action"`
		Method string            `json:"method"`
		URL    string            `json:"url"`
		Header map[string]string `json:"header"`
		Body   string            `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	ok := s.Proxy.Inter.ResolveExported(body.ID, body.Action, body.Method, body.URL, body.Header, body.Body)
	writeJSON(w, map[string]bool{"ok": ok})
}

func (s *Server) scope(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string][]string{"hosts": s.Proxy.Scope()})
	case http.MethodPut:
		var body struct {
			Hosts []string `json:"hosts"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		s.Proxy.SetScope(body.Hosts)
		writeJSON(w, map[string][]string{"hosts": s.Proxy.Scope()})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Op    string `json:"op"`
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	out, err := toolkit.Transform(body.Op, body.Input)
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]string{"output": out})
}

func (s *Server) automate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var spec automate.Spec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	report, err := automate.Run(s.Proxy.Transport(), spec)
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, report)
}

// sitemap builds a host/path tree from captured flows.
func (s *Server) sitemap(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	flows := s.Store.List(filter, 0)
	type node struct {
		Name     string           `json:"name"`
		Count    int              `json:"count"`
		Methods  map[string]bool  `json:"methods,omitempty"`
		Children map[string]*node `json:"children,omitempty"`
	}
	root := map[string]*node{}
	for _, f := range flows {
		host := f.Scheme + "://" + f.Host
		hn := root[host]
		if hn == nil {
			hn = &node{Name: host, Children: map[string]*node{}}
			root[host] = hn
		}
		hn.Count++
		path := f.Path
		if i := strings.IndexByte(path, '?'); i >= 0 {
			path = path[:i]
		}
		segs := strings.Split(strings.Trim(path, "/"), "/")
		cur := hn
		for _, seg := range segs {
			if seg == "" {
				continue
			}
			ch := cur.Children[seg]
			if ch == nil {
				ch = &node{Name: seg, Children: map[string]*node{}, Methods: map[string]bool{}}
				cur.Children[seg] = ch
			}
			ch.Count++
			ch.Methods[f.Method] = true
			cur = ch
		}
	}
	writeJSON(w, root)
}

// compare returns a line-level diff between two texts.
func (s *Server) compare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		A string `json:"a"`
		B string `json:"b"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]any{"diff": toolkit.DiffLines(body.A, body.B)})
}

func (s *Server) sequencer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Tokens []string `json:"tokens"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	writeJSON(w, toolkit.Sequence(body.Tokens))
}

func (s *Server) wsList(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		filter := r.URL.Query().Get("filter")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit == 0 {
			limit = 500
		}
		writeJSON(w, s.Proxy.WSStore.List(filter, limit))
	case http.MethodDelete:
		s.Proxy.WSStore.Clear()
		writeJSON(w, map[string]string{"status": "cleared"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) wsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch, cancel := s.Proxy.WSStore.Subscribe()
	defer cancel()
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case m := <-ch:
			b, _ := json.Marshal(m)
			fmt.Fprintf(w, "data: %s\n\n", b)
			flusher.Flush()
		}
	}
}

func (s *Server) upstream(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]string{"upstream": s.Proxy.Upstream()})
	case http.MethodPut:
		var body struct {
			Upstream string `json:"upstream"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		s.Proxy.SetUpstream(body.Upstream)
		writeJSON(w, map[string]string{"upstream": s.Proxy.Upstream()})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) annotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ID      int64  `json:"id"`
		Color   string `json:"color"`
		Note    string `json:"note"`
		Starred bool   `json:"starred"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	ok := s.Store.Annotate(body.ID, body.Color, body.Note, body.Starred)
	writeJSON(w, map[string]bool{"ok": ok})
}

func (s *Server) findings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.Findings.List())
	case http.MethodPost:
		var f store.Finding
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		writeJSON(w, s.Findings.Add(&f))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) findingByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/findings/"), 10, 64)
	s.Findings.Remove(id)
	writeJSON(w, map[string]string{"status": "removed"})
}

func (s *Server) presets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.Presets.List())
	case http.MethodPost:
		var p store.Preset
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		writeJSON(w, s.Presets.Add(&p))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) presetByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/presets/"), 10, 64)
	s.Presets.Remove(id)
	writeJSON(w, map[string]string{"status": "removed"})
}

// exportRule is the serializable form of a match/replace rule.
type exportRule struct {
	Enabled bool   `json:"enabled"`
	Name    string `json:"name"`
	Target  string `json:"target"`
	Match   string `json:"match"`
	Replace string `json:"replace"`
}

type projectFile struct {
	Version  int                `json:"version"`
	Flows    []*store.Flow      `json:"flows"`
	Findings []*store.Finding   `json:"findings"`
	Presets  []*store.Preset    `json:"presets"`
	Scope    []string           `json:"scope"`
	Rules    []exportRule       `json:"rules"`
	Upstream string             `json:"upstream"`
	WS       []*store.WSMessage `json:"ws,omitempty"`
}

func (s *Server) projectExport(w http.ResponseWriter, r *http.Request) {
	pf := projectFile{
		Version:  1,
		Flows:    s.Store.Snapshot(),
		Findings: s.Findings.Snapshot(),
		Presets:  s.Presets.List(),
		Scope:    s.Proxy.Scope(),
		Upstream: s.Proxy.Upstream(),
		WS:       s.Proxy.WSStore.List("", 0),
	}
	for _, rl := range s.Proxy.Rules.List() {
		pf.Rules = append(pf.Rules, exportRule{rl.Enabled, rl.Name, rl.Target, rl.Match, rl.Replace})
	}
	w.Header().Set("Content-Disposition", `attachment; filename="mytraffic-project.json"`)
	writeJSON(w, pf)
}

func (s *Server) projectImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var pf projectFile
	if err := json.NewDecoder(r.Body).Decode(&pf); err != nil {
		http.Error(w, "bad json: "+err.Error(), http.StatusBadRequest)
		return
	}
	s.Store.Load(pf.Flows)
	s.Findings.Load(pf.Findings)
	s.Presets.Load(pf.Presets)
	s.Proxy.SetScope(pf.Scope)
	s.Proxy.SetUpstream(pf.Upstream)
	for _, rl := range pf.Rules {
		s.Proxy.Rules.Add(&proxy.Rule{Enabled: rl.Enabled, Name: rl.Name, Target: rl.Target, Match: rl.Match, Replace: rl.Replace})
	}
	writeJSON(w, map[string]any{"flows": len(pf.Flows), "findings": len(pf.Findings), "status": "loaded"})
}

func (s *Server) caCert(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", `attachment; filename="myTraffic-ca.pem"`)
	w.Write(s.Proxy.CA.RootPEM())
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
