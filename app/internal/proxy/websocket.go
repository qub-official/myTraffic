package proxy

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qub-official/myTraffic/internal/store"
)

const maxWSRecord = 64 << 10 // cap stored payload text per frame

// isWSUpgrade reports whether r is a WebSocket upgrade handshake.
func isWSUpgrade(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") &&
		strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// handleWebSocket tunnels a WebSocket connection while recording each frame.
// client is the downstream connection; clientReader is its buffered reader
// (already positioned after the handshake request line/headers).
func (p *Proxy) handleWebSocket(client net.Conn, clientReader *bufio.Reader, req *http.Request, scheme, hostport string) {
	host := stripPort(hostport)
	port := wsPort(scheme, hostport)
	addr := hostport
	if !strings.Contains(addr, ":") {
		addr = net.JoinHostPort(addr, strconv.Itoa(port))
	}

	var up net.Conn
	var err error
	if scheme == "wss" {
		up, err = tls.Dial("tcp", addr, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	} else {
		up, err = net.Dial("tcp", addr)
	}
	if err != nil {
		return
	}
	defer up.Close()

	// Forward the handshake request upstream verbatim.
	if err := req.Write(up); err != nil {
		return
	}
	upReader := bufio.NewReader(up)

	// Read the upstream response head raw and relay it to the client.
	head, status, err := readHTTPHead(upReader)
	if err != nil {
		return
	}
	if _, err := client.Write(head); err != nil {
		return
	}
	if status != 101 {
		return
	}

	recording := p.inScope(host)
	rec := func(dir string, opcode byte, payload []byte) {
		if !recording {
			return
		}
		format, text := wsFormat(opcode, payload)
		p.WSStore.Add(&store.WSMessage{
			Time:      time.Now(),
			Scheme:    scheme,
			Host:      host,
			Path:      req.URL.RequestURI(),
			Port:      port,
			Direction: dir,
			Format:    format,
			Payload:   text,
		})
	}

	var wg sync.WaitGroup
	wg.Add(2)
	// client -> server
	go func() {
		defer wg.Done()
		relayFrames(clientReader, up, "client", rec)
		up.Close()
	}()
	// server -> client
	go func() {
		defer wg.Done()
		relayFrames(upReader, client, "server", rec)
		client.Close()
	}()
	wg.Wait()
}

// relayFrames reads WebSocket frames from src, writes the exact bytes to dst,
// and reports each frame's opcode and unmasked payload to rec.
func relayFrames(src *bufio.Reader, dst io.Writer, dir string, rec func(string, byte, []byte)) {
	for {
		raw, opcode, payload, err := readFrame(src)
		if err != nil {
			return
		}
		if _, err := dst.Write(raw); err != nil {
			return
		}
		rec(dir, opcode, payload)
		if opcode == 0x8 { // close
			return
		}
	}
}

// readFrame reads one frame, returning its exact bytes, opcode, and the
// unmasked payload (for recording). Based on RFC 6455.
func readFrame(r *bufio.Reader) (raw []byte, opcode byte, payload []byte, err error) {
	b0, err := r.ReadByte()
	if err != nil {
		return nil, 0, nil, err
	}
	b1, err := r.ReadByte()
	if err != nil {
		return nil, 0, nil, err
	}
	raw = []byte{b0, b1}
	opcode = b0 & 0x0f
	masked := b1&0x80 != 0
	length := uint64(b1 & 0x7f)

	switch length {
	case 126:
		ext := make([]byte, 2)
		if _, err = io.ReadFull(r, ext); err != nil {
			return
		}
		raw = append(raw, ext...)
		length = uint64(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		if _, err = io.ReadFull(r, ext); err != nil {
			return
		}
		raw = append(raw, ext...)
		length = binary.BigEndian.Uint64(ext)
	}

	var mask []byte
	if masked {
		mask = make([]byte, 4)
		if _, err = io.ReadFull(r, mask); err != nil {
			return
		}
		raw = append(raw, mask...)
	}

	body := make([]byte, length)
	if _, err = io.ReadFull(r, body); err != nil {
		return
	}
	raw = append(raw, body...)

	payload = body
	if masked {
		payload = make([]byte, length)
		for i := range body {
			payload[i] = body[i] ^ mask[i%4]
		}
	}
	return raw, opcode, payload, nil
}

// readHTTPHead reads an HTTP message head (up to and including the blank line)
// and returns the raw bytes plus the status code (0 for a request).
func readHTTPHead(r *bufio.Reader) ([]byte, int, error) {
	var buf []byte
	status := 0
	first := true
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return nil, 0, err
		}
		buf = append(buf, line...)
		if first {
			first = false
			parts := strings.SplitN(strings.TrimSpace(string(line)), " ", 3)
			if len(parts) >= 2 {
				status, _ = strconv.Atoi(parts[1])
			}
		}
		if string(line) == "\r\n" || string(line) == "\n" {
			break
		}
	}
	return buf, status, nil
}

func wsFormat(opcode byte, payload []byte) (format, text string) {
	switch opcode {
	case 0x1:
		format = "Text"
		text = capStr(string(payload))
	case 0x2:
		format = "Binary"
		text = base64.StdEncoding.EncodeToString(capBytes(payload))
	case 0x8:
		format = "Close"
	case 0x9:
		format = "Ping"
		text = capStr(string(payload))
	case 0xA:
		format = "Pong"
		text = capStr(string(payload))
	default:
		format = "Continuation"
		text = capStr(string(payload))
	}
	return format, text
}

func capStr(s string) string {
	if len(s) > maxWSRecord {
		return s[:maxWSRecord] + "…"
	}
	return s
}
func capBytes(b []byte) []byte {
	if len(b) > maxWSRecord {
		return b[:maxWSRecord]
	}
	return b
}

func wsPort(scheme, hostport string) int {
	if _, p, err := net.SplitHostPort(hostport); err == nil {
		if n, err := strconv.Atoi(p); err == nil {
			return n
		}
	}
	if scheme == "wss" {
		return 443
	}
	return 80
}
