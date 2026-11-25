// Package proxy implements an intercepting HTTP/HTTPS proxy. It records every
// flow, applies match/replace rules, supports scope filtering and manual
// interception, and mints TLS certificates on the fly for HTTPS inspection.
package proxy

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"strings"
	"sync"
	"time"

	"github.com/qub-official/myTraffic/internal/ca"
	"github.com/qub-official/myTraffic/internal/store"
)

const maxBody = 5 << 20 // 5 MiB captured per message

// Proxy is an intercepting forward proxy.
type Proxy struct {
	CA      *ca.CA
	Store   *store.Store
	WSStore *store.WSStore
	Rules   *RuleSet
	Inter   *Interceptor

	transport *http.Transport

	mu       sync.RWMutex
	scope    []string // host suffixes; empty means "everything"
	upstream string   // optional upstream proxy URL
}

// New builds a proxy wired to the given CA and store.
func New(c *ca.CA, st *store.Store) *Proxy {
	p := &Proxy{
		CA:      c,
		Store:   st,
		WSStore: store.NewWSStore(0),
		Rules:   NewRuleSet(),
		Inter:   NewInterceptor(),
	}
	p.transport = &http.Transport{
		Proxy:               p.proxyForRequest,
		ForceAttemptHTTP2:   false,
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 15 * time.Second,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
	}
	return p
}

// SetUpstream routes all outbound traffic through another proxy (e.g.
// http://127.0.0.1:8081), or clears it when u is empty.
func (p *Proxy) SetUpstream(u string) {
	p.mu.Lock()
	p.upstream = u
	p.mu.Unlock()
}

// Upstream returns the configured upstream proxy URL.
func (p *Proxy) Upstream() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.upstream
}

// proxyForRequest selects the upstream proxy for a request: the configured
// upstream if set, otherwise the environment's proxy settings.
func (p *Proxy) proxyForRequest(req *http.Request) (*neturl.URL, error) {
	p.mu.RLock()
	up := p.upstream
	p.mu.RUnlock()
	if up != "" {
		return neturl.Parse(up)
	}
	return http.ProxyFromEnvironment(req)
}

// Transport returns the proxy's HTTP transport so other components (Automate,
// Replay) can send requests through the same connection pool and TLS config.
func (p *Proxy) Transport() http.RoundTripper { return p.transport }

// SetScope limits recording/interception to the given host suffixes.
func (p *Proxy) SetScope(hosts []string) {
	p.mu.Lock()
	p.scope = hosts
	p.mu.Unlock()
}

// Scope returns the current scope.
func (p *Proxy) Scope() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, len(p.scope))
	copy(out, p.scope)
	return out
}

func (p *Proxy) inScope(host string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.scope) == 0 {
		return true
	}
	host = stripPort(host)
	for _, s := range p.scope {
		if s == "" {
			continue
		}
		if host == s || strings.HasSuffix(host, "."+s) {
			return true
		}
	}
	return false
}

// ServeHTTP is the proxy entry point.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handlePlain(w, r)
}

// handlePlain forwards an ordinary (non-CONNECT) HTTP request.
func (p *Proxy) handlePlain(w http.ResponseWriter, r *http.Request) {
	if isWSUpgrade(r) {
		p.handlePlainWS(w, r)
		return
	}
	p.forward(w, r, "http")
}

// handlePlainWS hijacks a plain-HTTP WebSocket upgrade and tunnels it.
func (p *Proxy) handlePlainWS(w http.ResponseWriter, r *http.Request) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	host := r.Host
	r.URL.Scheme = "http"
	r.URL.Host = host
	p.handleWebSocket(conn, buf.Reader, r, "ws", host)
}

// handleConnect establishes a MITM TLS tunnel and serves requests off it.
func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	host := stripPort(r.Host)
	leaf, err := p.CA.Leaf(host)
	if err != nil {
		http.Error(w, "cert error", http.StatusServiceUnavailable)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()
	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return
	}

	tlsConn := tls.Server(clientConn, &tls.Config{
		Certificates: []tls.Certificate{*leaf},
		MinVersion:   tls.VersionTLS12,
	})
	if err := tlsConn.Handshake(); err != nil {
		return
	}
	defer tlsConn.Close()

	reader := bufio.NewReader(tlsConn)
	for {
		req, err := http.ReadRequest(reader)
		if err != nil {
			return
		}
		req.URL.Scheme = "https"
		req.URL.Host = r.Host
		req.RemoteAddr = clientConn.RemoteAddr().String()

		if isWSUpgrade(req) {
			p.handleWebSocket(tlsConn, reader, req, "wss", r.Host)
			return
		}

		rec := &connResponseWriter{conn: tlsConn}
		p.forward(rec, req, "https")
		if req.Close || rec.closed {
			return
		}
	}
}

// forward performs rule rewriting, optional interception, upstream round-trip,
// recording, and writing the response back to the client.
func (p *Proxy) forward(w http.ResponseWriter, r *http.Request, scheme string) {
	start := time.Now()
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	recording := p.inScope(host)

	// Read and cap the request body.
	reqBody := readBody(&r.Body)

	// Apply request match/replace rules.
	if nb, changed := p.Rules.apply("req.body", reqBody); changed {
		reqBody = nb
	}
	rewriteHeaders(r.Header, p.Rules, "req.header")

	// Manual interception.
	if recording && p.Inter.Enabled() {
		d := p.Inter.hold(r.Method, fullURL(scheme, host, r.URL.RequestURI()), headerMap(r.Header), reqBody)
		if d.action == "drop" {
			http.Error(w, "dropped by myTraffic", http.StatusForbidden)
			return
		}
		if d.body != "" {
			reqBody = d.body
		}
		if len(d.header) > 0 {
			applyHeaderMap(r.Header, d.header)
		}
	}

	// Build the upstream request.
	outURL := fullURL(scheme, host, r.URL.RequestURI())
	out, err := http.NewRequest(r.Method, outURL, strings.NewReader(reqBody))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadGateway)
		return
	}
	copyHeaders(out.Header, r.Header)
	out.Header.Del("Proxy-Connection")

	resp, err := p.transport.RoundTrip(out)
	if err != nil {
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
		if recording {
			p.record(scheme, host, r, reqBody, 502, nil, err.Error(), time.Since(start))
		}
		return
	}
	defer resp.Body.Close()

	resBody := readBody(&resp.Body)
	if nb, changed := p.Rules.apply("resp.body", resBody); changed {
		resBody = nb
	}
	rewriteHeaders(resp.Header, p.Rules, "resp.header")

	// Write response to client.
	for k, vs := range resp.Header {
		if strings.EqualFold(k, "Content-Length") {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("Content-Length", itoa(len(resBody)))
	w.WriteHeader(resp.StatusCode)
	io.WriteString(w, resBody)

	if recording {
		p.record(scheme, host, r, reqBody, resp.StatusCode, resp.Header, resBody, time.Since(start))
	}
}

func (p *Proxy) record(scheme, host string, r *http.Request, reqBody string, status int, resHeaders http.Header, resBody string, dur time.Duration) {
	p.Store.Add(&store.Flow{
		Time:       time.Now(),
		Method:     r.Method,
		Scheme:     scheme,
		Host:       stripPort(host),
		Path:       r.URL.RequestURI(),
		ReqHeaders: headerMap(r.Header),
		ReqBody:    reqBody,
		Status:     status,
		ResHeaders: headerMap(resHeaders),
		ResBody:    resBody,
		Size:       len(resBody),
		DurationMs: dur.Milliseconds(),
	})
}

// connResponseWriter adapts a raw TLS connection to http.ResponseWriter so the
// same forward() path serves both plain and tunneled requests.
type connResponseWriter struct {
	conn        net.Conn
	header      http.Header
	wroteHeader bool
	closed      bool
}

func (c *connResponseWriter) Header() http.Header {
	if c.header == nil {
		c.header = make(http.Header)
	}
	return c.header
}

func (c *connResponseWriter) WriteHeader(status int) {
	if c.wroteHeader {
		return
	}
	c.wroteHeader = true
	var b strings.Builder
	b.WriteString("HTTP/1.1 ")
	b.WriteString(itoa(status))
	b.WriteString(" ")
	b.WriteString(http.StatusText(status))
	b.WriteString("\r\n")
	for k, vs := range c.header {
		for _, v := range vs {
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(v)
			b.WriteString("\r\n")
		}
	}
	b.WriteString("Connection: keep-alive\r\n\r\n")
	if _, err := c.conn.Write([]byte(b.String())); err != nil {
		c.closed = true
	}
}

func (c *connResponseWriter) Write(b []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	n, err := c.conn.Write(b)
	if err != nil {
		c.closed = true
	}
	return n, err
}
