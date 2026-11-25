package proxy

import (
	"bufio"
	"bytes"
	"testing"
)

// buildFrame constructs a WebSocket frame for testing.
func buildFrame(opcode byte, payload []byte, mask []byte) []byte {
	var b bytes.Buffer
	b.WriteByte(0x80 | opcode) // FIN + opcode
	n := len(payload)
	masked := byte(0)
	if mask != nil {
		masked = 0x80
	}
	switch {
	case n < 126:
		b.WriteByte(masked | byte(n))
	case n < 65536:
		b.WriteByte(masked | 126)
		b.WriteByte(byte(n >> 8))
		b.WriteByte(byte(n))
	}
	if mask != nil {
		b.Write(mask)
		masked := make([]byte, n)
		for i := range payload {
			masked[i] = payload[i] ^ mask[i%4]
		}
		b.Write(masked)
	} else {
		b.Write(payload)
	}
	return b.Bytes()
}

func TestReadFrameMasked(t *testing.T) {
	mask := []byte{0x01, 0x02, 0x03, 0x04}
	frame := buildFrame(0x1, []byte("hello websocket"), mask)
	r := bufio.NewReader(bytes.NewReader(frame))
	raw, opcode, payload, err := readFrame(r)
	if err != nil {
		t.Fatal(err)
	}
	if opcode != 0x1 {
		t.Fatalf("opcode=%x want 1", opcode)
	}
	if string(payload) != "hello websocket" {
		t.Fatalf("payload=%q", payload)
	}
	if !bytes.Equal(raw, frame) {
		t.Fatalf("raw bytes not preserved exactly")
	}
}

func TestReadFrameUnmasked(t *testing.T) {
	frame := buildFrame(0x2, bytes.Repeat([]byte("A"), 200), nil) // 200 bytes -> extended length
	r := bufio.NewReader(bytes.NewReader(frame))
	raw, opcode, payload, err := readFrame(r)
	if err != nil {
		t.Fatal(err)
	}
	if opcode != 0x2 {
		t.Fatalf("opcode=%x want 2", opcode)
	}
	if len(payload) != 200 {
		t.Fatalf("payload len=%d want 200", len(payload))
	}
	if !bytes.Equal(raw, frame) {
		t.Fatalf("raw bytes not preserved")
	}
}

func TestWSFormat(t *testing.T) {
	if f, txt := wsFormat(0x1, []byte("hi")); f != "Text" || txt != "hi" {
		t.Fatalf("text frame: %s %s", f, txt)
	}
	if f, _ := wsFormat(0x8, nil); f != "Close" {
		t.Fatalf("close frame: %s", f)
	}
}

func TestReadHTTPHead(t *testing.T) {
	resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n"
	r := bufio.NewReader(bytes.NewReader([]byte(resp)))
	head, status, err := readHTTPHead(r)
	if err != nil {
		t.Fatal(err)
	}
	if status != 101 {
		t.Fatalf("status=%d want 101", status)
	}
	if string(head) != resp {
		t.Fatalf("head mismatch")
	}
}
