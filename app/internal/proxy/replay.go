package proxy

import (
	"io"
	"net/http"
	"strings"
	"time"
)

// ReplayReq describes a request to re-send.
type ReplayReq struct {
	Method string            `json:"method"`
	URL    string            `json:"url"`
	Header map[string]string `json:"header"`
	Body   string            `json:"body"`
}

// ReplayResp is the result of a replay.
type ReplayResp struct {
	Status     int               `json:"status"`
	Header     map[string]string `json:"header"`
	Body       string            `json:"body"`
	DurationMs int64             `json:"durationMs"`
}

// Replay sends a one-off request through the proxy's transport and returns the
// response without recording it as a captured flow.
func (p *Proxy) Replay(rr ReplayReq) (*ReplayResp, error) {
	method := rr.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequest(method, rr.URL, strings.NewReader(rr.Body))
	if err != nil {
		return nil, err
	}
	for k, v := range rr.Header {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := p.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	return &ReplayResp{
		Status:     resp.StatusCode,
		Header:     headerMap(resp.Header),
		Body:       string(body),
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// ReplayFlow re-sends a previously captured flow by ID.
func (p *Proxy) ReplayFlow(id int64) (*ReplayResp, error) {
	f, ok := p.Store.Get(id)
	if !ok {
		return nil, errNotFound
	}
	return p.Replay(ReplayReq{
		Method: f.Method,
		URL:    f.Scheme + "://" + f.Host + f.Path,
		Header: f.ReqHeaders,
		Body:   f.ReqBody,
	})
}
