// Package mcp implements a minimal Model Context Protocol server over stdio so
// AI assistants can query captured traffic and replay requests. It speaks
// JSON-RPC 2.0 with newline-delimited messages (the MCP stdio convention) and
// forwards each tool call to a running myTraffic instance's local HTTP API, so
// it shares the same live history as the desktop app.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const protocolVersion = "2024-11-05"

// Server forwards MCP tool calls to the local API at BaseURL.
type Server struct {
	BaseURL string
	client  *http.Client
}

// New creates a server targeting the given API base URL (e.g. http://127.0.0.1:8081).
func New(baseURL string) *Server {
	return &Server{
		BaseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResp struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ServeStdio runs the stdio loop on the process's stdin/stdout until EOF.
func (s *Server) ServeStdio() error { return s.Serve(os.Stdin, os.Stdout) }

// Serve runs the JSON-RPC loop.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(out)

	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var req rpcReq
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		resp := s.dispatch(req)
		if resp == nil {
			continue
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (s *Server) dispatch(req rpcReq) *rpcResp {
	switch req.Method {
	case "initialize":
		return ok(req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "myTraffic", "version": "1.0.0"},
		})
	case "notifications/initialized":
		return nil
	case "ping":
		return ok(req.ID, map[string]any{})
	case "tools/list":
		return ok(req.ID, map[string]any{"tools": toolDefs()})
	case "tools/call":
		return s.callTool(req)
	default:
		if req.ID == nil {
			return nil
		}
		return fail(req.ID, -32601, "method not found: "+req.Method)
	}
}

func (s *Server) callTool(req rpcReq) *rpcResp {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	json.Unmarshal(req.Params, &p)

	switch p.Name {
	case "list_flows":
		var a struct {
			Filter string `json:"filter"`
			Limit  int    `json:"limit"`
		}
		json.Unmarshal(p.Arguments, &a)
		if a.Limit == 0 {
			a.Limit = 50
		}
		q := url.Values{}
		q.Set("filter", a.Filter)
		q.Set("limit", fmt.Sprintf("%d", a.Limit))
		return s.proxyGet(req.ID, "/api/flows?"+q.Encode())
	case "get_flow":
		var a struct {
			ID int64 `json:"id"`
		}
		json.Unmarshal(p.Arguments, &a)
		return s.proxyGet(req.ID, fmt.Sprintf("/api/flows/%d", a.ID))
	case "replay_flow":
		var a struct {
			ID int64 `json:"id"`
		}
		json.Unmarshal(p.Arguments, &a)
		return s.proxyPost(req.ID, "/api/replay", map[string]any{"id": a.ID})
	case "send_request":
		var body map[string]any
		json.Unmarshal(p.Arguments, &body)
		return s.proxyPost(req.ID, "/api/replay", body)
	case "decode":
		var body map[string]any
		json.Unmarshal(p.Arguments, &body)
		return s.proxyPost(req.ID, "/api/decode", body)
	case "set_intercept":
		var body map[string]any
		json.Unmarshal(p.Arguments, &body)
		return s.proxyPost(req.ID, "/api/intercept", body)
	case "get_scope":
		return s.proxyGet(req.ID, "/api/scope")
	case "automate":
		var body map[string]any
		json.Unmarshal(p.Arguments, &body)
		return s.proxyPost(req.ID, "/api/automate", body)
	case "sitemap":
		return s.proxyGet(req.ID, "/api/sitemap")
	case "list_ws":
		var a struct {
			Filter string `json:"filter"`
			Limit  int    `json:"limit"`
		}
		json.Unmarshal(p.Arguments, &a)
		if a.Limit == 0 {
			a.Limit = 50
		}
		q := url.Values{}
		q.Set("filter", a.Filter)
		q.Set("limit", fmt.Sprintf("%d", a.Limit))
		return s.proxyGet(req.ID, "/api/ws?"+q.Encode())
	case "sequencer":
		var body map[string]any
		json.Unmarshal(p.Arguments, &body)
		return s.proxyPost(req.ID, "/api/sequencer", body)
	case "list_findings":
		return s.proxyGet(req.ID, "/api/findings")
	case "add_finding":
		var body map[string]any
		json.Unmarshal(p.Arguments, &body)
		return s.proxyPost(req.ID, "/api/findings", body)
	default:
		return fail(req.ID, -32602, "unknown tool: "+p.Name)
	}
}

func (s *Server) proxyGet(id json.RawMessage, path string) *rpcResp {
	resp, err := s.client.Get(s.BaseURL + path)
	if err != nil {
		return text(id, "myTraffic is not running on "+s.BaseURL+": "+err.Error())
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return text(id, string(b))
}

func (s *Server) proxyPost(id json.RawMessage, path string, body any) *rpcResp {
	b, _ := json.Marshal(body)
	resp, err := s.client.Post(s.BaseURL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return text(id, "myTraffic is not running on "+s.BaseURL+": "+err.Error())
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	return text(id, string(rb))
}

func toolDefs() []map[string]any {
	str := map[string]any{"type": "string"}
	num := map[string]any{"type": "integer"}
	boolean := map[string]any{"type": "boolean"}
	obj := func(props map[string]any, required ...string) map[string]any {
		m := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			m["required"] = required
		}
		return m
	}
	return []map[string]any{
		{"name": "list_flows", "description": "List captured HTTP flows. Optional HTTPQL-style filter, e.g. host.cont:\"api\" resp.code.gte:400", "inputSchema": obj(map[string]any{"filter": str, "limit": num})},
		{"name": "get_flow", "description": "Get one captured flow by numeric id, with headers and bodies.", "inputSchema": obj(map[string]any{"id": num}, "id")},
		{"name": "replay_flow", "description": "Re-send a captured flow by id and return the fresh response.", "inputSchema": obj(map[string]any{"id": num}, "id")},
		{"name": "send_request", "description": "Send a new HTTP request through the proxy and return the response.", "inputSchema": obj(map[string]any{"method": str, "url": str, "header": map[string]any{"type": "object"}, "body": str}, "url")},
		{"name": "decode", "description": "Transform text: base64encode/decode, urlencode/decode, hexencode/decode, md5, sha1, sha256, jwtdecode.", "inputSchema": obj(map[string]any{"op": str, "input": str}, "op", "input")},
		{"name": "set_intercept", "description": "Turn manual request interception on or off.", "inputSchema": obj(map[string]any{"enabled": boolean}, "enabled")},
		{"name": "get_scope", "description": "Get the current in-scope host list.", "inputSchema": obj(map[string]any{})},
		{"name": "sitemap", "description": "Get the host/path tree built from captured traffic.", "inputSchema": obj(map[string]any{})},
		{"name": "list_ws", "description": "List captured WebSocket messages. Optional StreamQL filter, e.g. ws.direction.eq:\"client\".", "inputSchema": obj(map[string]any{"filter": str, "limit": num})},
		{"name": "sequencer", "description": "Analyze randomness/entropy of a set of tokens.", "inputSchema": obj(map[string]any{"tokens": map[string]any{"type": "array", "items": str}}, "tokens")},
		{"name": "list_findings", "description": "List recorded findings.", "inputSchema": obj(map[string]any{})},
		{"name": "add_finding", "description": "Record a finding.", "inputSchema": obj(map[string]any{"title": str, "severity": str, "host": str, "flowId": num, "note": str}, "title")},
		{"name": "automate", "description": "Run a fuzzing attack (Intruder/Automate). Mark positions in url/body/header values with § markers. attackType: sniper|batteringram|pitchfork|clusterbomb. payloads is an array of sets, each {type:list|numbers|null|brute, ...}.", "inputSchema": obj(map[string]any{
			"method":     str,
			"url":        str,
			"headers":    map[string]any{"type": "object"},
			"body":       str,
			"attackType": str,
			"payloads":   map[string]any{"type": "array"},
			"grep":       map[string]any{"type": "array", "items": str},
		}, "url", "payloads")},
	}
}

func ok(id json.RawMessage, result any) *rpcResp {
	return &rpcResp{JSONRPC: "2.0", ID: id, Result: result}
}

func fail(id json.RawMessage, code int, msg string) *rpcResp {
	return &rpcResp{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

func text(id json.RawMessage, s string) *rpcResp {
	return ok(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": s}},
	})
}
