// Command mytraffic is a lightweight, local-first intercepting proxy with a web
// UI, a CLI, and an MCP server. Everything runs on localhost; nothing is sent
// to any remote service.
//
// Usage:
//
//	mytraffic proxy                 run the proxy + web UI (default)
//	mytraffic mcp                   run the MCP server (stdio) for AI clients
//	mytraffic history [--filter f]  print captured flows from a running instance
//	mytraffic replay <id>           replay a captured flow
//	mytraffic decode <op> <input>   run a decoder transform
//	mytraffic ca                    print the path to the root CA certificate
//	mytraffic version
package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/qub-official/myTraffic/internal/api"
	"github.com/qub-official/myTraffic/internal/ca"
	"github.com/qub-official/myTraffic/internal/mcp"
	"github.com/qub-official/myTraffic/internal/proxy"
	"github.com/qub-official/myTraffic/internal/store"
	"github.com/qub-official/myTraffic/internal/toolkit"
)

const version = "1.0.0"

//go:embed web
var webFS embed.FS

func main() {
	args := os.Args[1:]
	cmd := "proxy"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd = args[0]
		args = args[1:]
	}

	var err error
	switch cmd {
	case "proxy", "run", "serve":
		err = cmdProxy(args)
	case "mcp":
		err = cmdMCP(args)
	case "history", "ls":
		err = cmdHistory(args)
	case "replay":
		err = cmdReplay(args)
	case "automate", "intruder", "fuzz":
		err = cmdAutomate(args)
	case "sequencer", "sequence":
		err = cmdSequencer(args)
	case "project":
		err = cmdProject(args)
	case "decode":
		err = cmdDecode(args)
	case "ca":
		err = cmdCA(args)
	case "version", "-v", "--version":
		fmt.Println("myTraffic", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`myTraffic ` + version + ` — local intercepting proxy

Commands:
  proxy     Run the proxy and web UI (default)
  mcp       Run the MCP server over stdio for AI assistants
  history   List captured flows from a running instance
  replay    Replay a captured flow by id
  automate  Run a fuzzing attack (Intruder/Automate)
  sequencer Analyze token randomness (entropy)
  project   Save or load a project: project export|import <file>
  decode    Transform text (base64, url, hex, md5, sha1, sha256, jwt)
  ca        Print the path to the root CA certificate to trust
  version   Print the version

Run "mytraffic proxy --help" for proxy options.
`)
}

func configDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base, _ = os.UserHomeDir()
	}
	dir := filepath.Join(base, "myTraffic")
	os.MkdirAll(dir, 0o700)
	return dir
}

// ---- proxy command ----

func cmdProxy(args []string) error {
	cfg := loadConfig()
	fsflags := newFlagSet(args)
	proxyAddr := fsflags.String("proxy-addr", cfg.ProxyAddr, "address for the intercepting proxy")
	apiAddr := fsflags.String("api-addr", cfg.APIAddr, "address for the web UI and API")
	persist := fsflags.String("persist", "", "append captured flows to this JSONL file")
	upstream := fsflags.String("upstream", cfg.Upstream, "route outbound traffic through another proxy URL")
	open := fsflags.Bool("open", cfg.OpenBrowser, "open the web UI in a browser")
	fsflags.parse()

	dir := configDir()
	caMgr, err := ca.Load(filepath.Join(dir, "ca"))
	if err != nil {
		return fmt.Errorf("loading CA: %w", err)
	}
	st := store.New(20000)
	if *persist != "" {
		if err := st.Persist(*persist); err != nil {
			return fmt.Errorf("persist: %w", err)
		}
		defer st.Close()
	}
	px := proxy.New(caMgr, st)
	if *upstream != "" {
		px.SetUpstream(*upstream)
	}
	if len(cfg.Scope) > 0 {
		px.SetScope(cfg.Scope)
	}

	// Web UI + API server.
	sub, _ := fs.Sub(webFS, "web")
	apiSrv := &api.Server{Proxy: px, Store: st, Findings: store.NewFindings(), Presets: store.NewPresets(), Static: sub}
	uiServer := &http.Server{Addr: *apiAddr, Handler: apiSrv.Handler()}

	// Proxy server.
	proxyServer := &http.Server{Addr: *proxyAddr, Handler: px}

	go func() {
		if err := uiServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "ui server:", err)
		}
	}()
	go func() {
		if err := proxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "proxy server:", err)
		}
	}()

	uiURL := "http://" + *apiAddr + "/"
	fmt.Println("myTraffic", version)
	fmt.Println("  proxy listening on   ", *proxyAddr)
	fmt.Println("  web UI on            ", uiURL)
	fmt.Println("  CA certificate dir   ", filepath.Join(dir, "ca"))
	fmt.Println("  set your browser/system HTTP(S) proxy to", *proxyAddr)
	fmt.Println("  then trust myTraffic-ca.pem to inspect HTTPS")
	if *open {
		time.Sleep(300 * time.Millisecond)
		openBrowser(uiURL)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
	fmt.Println("\nshutting down...")
	proxyServer.Close()
	uiServer.Close()
	return nil
}

// ---- mcp command ----

func cmdMCP(args []string) error {
	fsflags := newFlagSet(args)
	apiBase := fsflags.String("api", "http://127.0.0.1:8081", "base URL of a running myTraffic API")
	fsflags.parse()
	return mcp.New(*apiBase).ServeStdio()
}

// ---- history command ----

func cmdHistory(args []string) error {
	fsflags := newFlagSet(args)
	filter := fsflags.String("filter", "", "HTTPQL-style filter")
	limit := fsflags.Int("limit", 50, "max rows")
	apiBase := fsflags.String("api", "http://127.0.0.1:8081", "running myTraffic API")
	jsonOut := fsflags.Bool("json", false, "print raw JSON")
	fsflags.parse()

	u := fmt.Sprintf("%s/api/flows?limit=%d&filter=%s", strings.TrimRight(*apiBase, "/"), *limit, url(*filter))
	body, err := httpGet(u)
	if err != nil {
		return err
	}
	if *jsonOut {
		fmt.Println(string(body))
		return nil
	}
	var flows []store.Flow
	if err := json.Unmarshal(body, &flows); err != nil {
		return err
	}
	for _, f := range flows {
		fmt.Printf("%-5d %-6s %-3d %6dB %5dms  %s%s\n", f.ID, f.Method, f.Status, f.Size, f.DurationMs, f.Host, f.Path)
	}
	return nil
}

// ---- replay command ----

func cmdReplay(args []string) error {
	// Pull the first bare (non-flag) argument out as the id so it can appear
	// before or after flags, e.g. "replay 12" or "replay 12 --api ...".
	idStr, flagArgs := extractPositional(args)
	fsflags := newFlagSet(flagArgs)
	apiBase := fsflags.String("api", "http://127.0.0.1:8081", "running myTraffic API")
	fsflags.parse()
	if idStr == "" {
		return fmt.Errorf("usage: mytraffic replay <id> [--api URL]")
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return fmt.Errorf("bad id: %s", idStr)
	}
	body, err := httpPost(strings.TrimRight(*apiBase, "/")+"/api/replay", map[string]any{"id": id})
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

// ---- automate command ----

func cmdAutomate(args []string) error {
	fsflags := newFlagSet(args)
	url := fsflags.String("url", "", "target URL, with §marker§ positions")
	method := fsflags.String("method", "GET", "HTTP method")
	attack := fsflags.String("attack", "sniper", "sniper|batteringram|pitchfork|clusterbomb")
	wordlist := fsflags.String("wordlist", "", "payload file (one per line) for a single set")
	body := fsflags.String("body", "", "request body, with §marker§ positions")
	grep := fsflags.String("grep", "", "flag responses containing this string")
	conc := fsflags.Int("concurrency", 10, "concurrent requests")
	apiBase := fsflags.String("api", "http://127.0.0.1:8081", "running myTraffic API")
	fsflags.parse()

	if *url == "" {
		return fmt.Errorf("usage: mytraffic automate --url 'https://host/§FUZZ§' --wordlist words.txt")
	}
	var list []string
	if *wordlist != "" {
		b, err := os.ReadFile(*wordlist)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
			if line != "" {
				list = append(list, line)
			}
		}
	}
	spec := map[string]any{
		"method":      *method,
		"url":         *url,
		"body":        *body,
		"attackType":  *attack,
		"concurrency": *conc,
		"payloads":    []map[string]any{{"type": "list", "list": list}},
	}
	if *grep != "" {
		spec["grep"] = []string{*grep}
	}
	data, err := httpPost(strings.TrimRight(*apiBase, "/")+"/api/automate", spec)
	if err != nil {
		return err
	}
	// Pretty-print a compact results table.
	var report struct {
		Count   int `json:"count"`
		Results []struct {
			Index      int             `json:"index"`
			Payloads   []string        `json:"payloads"`
			Status     int             `json:"status"`
			Length     int             `json:"length"`
			DurationMs int64           `json:"durationMs"`
			Grep       map[string]bool `json:"grep"`
			Error      string          `json:"error"`
		} `json:"results"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		fmt.Println(string(data))
		return nil
	}
	if report.Error != "" {
		return fmt.Errorf("%s", report.Error)
	}
	fmt.Printf("%-5s %-4s %-8s %-7s %s\n", "#", "code", "length", "time", "payload")
	for _, r := range report.Results {
		mark := ""
		for g, hit := range r.Grep {
			if hit {
				mark = " ← " + g
			}
		}
		pl := strings.Join(r.Payloads, ",")
		if r.Error != "" {
			pl = "ERR: " + r.Error
		}
		fmt.Printf("%-5d %-4d %-8d %-5dms %s%s\n", r.Index, r.Status, r.Length, r.DurationMs, pl, mark)
	}
	fmt.Printf("%d requests\n", report.Count)
	return nil
}

// ---- sequencer command ----

func cmdSequencer(args []string) error {
	fsflags := newFlagSet(args)
	file := fsflags.String("file", "", "token file, one per line (default stdin)")
	apiBase := fsflags.String("api", "http://127.0.0.1:8081", "running myTraffic API")
	fsflags.parse()

	var raw []byte
	var err error
	if *file != "" {
		raw, err = os.ReadFile(*file)
	} else {
		raw, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		return err
	}
	var tokens []string
	for _, l := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			tokens = append(tokens, l)
		}
	}
	data, err := httpPost(strings.TrimRight(*apiBase, "/")+"/api/sequencer", map[string]any{"tokens": tokens})
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// ---- project command ----

func cmdProject(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: mytraffic project export|import <file> [--api URL]")
	}
	sub := args[0]
	path, flagArgs := extractPath(args[1:])
	fsflags := newFlagSet(flagArgs)
	apiBase := fsflags.String("api", "http://127.0.0.1:8081", "running myTraffic API")
	fsflags.parse()
	if path == "" {
		return fmt.Errorf("usage: mytraffic project %s <file>", sub)
	}
	base := strings.TrimRight(*apiBase, "/")
	switch sub {
	case "export", "save":
		data, err := httpGet(base + "/api/project/export")
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return err
		}
		fmt.Println("saved project to", path)
	case "import", "load":
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		resp, err := http.Post(base+"/api/project/import", "application/json", bytes.NewReader(raw))
		if err != nil {
			return fmt.Errorf("is myTraffic running? %w", err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		fmt.Println(string(out))
	default:
		return fmt.Errorf("unknown project action %q (use export or import)", sub)
	}
	return nil
}

// ---- decode command ----

func cmdDecode(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: mytraffic decode <op> <input>\nops: %s", strings.Join(toolkit.Ops(), ", "))
	}
	out, err := toolkit.Transform(args[0], strings.Join(args[1:], " "))
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

// ---- ca command ----

func cmdCA(args []string) error {
	path := filepath.Join(configDir(), "ca", "myTraffic-ca.pem")
	if _, err := os.Stat(path); err != nil {
		// Generate it by loading the CA.
		if _, err := ca.Load(filepath.Join(configDir(), "ca")); err != nil {
			return err
		}
	}
	fmt.Println(path)
	return nil
}

// ---- helpers ----

func httpGet(u string) ([]byte, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, fmt.Errorf("is myTraffic running? %w", err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func httpPost(u string, body any) ([]byte, error) {
	b, _ := json.Marshal(body)
	resp, err := http.Post(u, "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("is myTraffic running? %w", err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func url(s string) string { return strings.ReplaceAll(s, " ", "%20") }

func openBrowser(u string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", u}
	case "darwin":
		cmd, args = "open", []string{u}
	default:
		cmd, args = "xdg-open", []string{u}
	}
	_ = exec.Command(cmd, args...).Start()
}
