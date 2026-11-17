package api

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Frame opcodes defined in RFC 6455.
const (
	OpContinuation = 0x0
	OpText         = 0x1
	OpBinary       = 0x2
	OpClose        = 0x8
	OpPing         = 0x9
	OpPong         = 0xA
)

// Conn represents a standard WebSocket connection over an underlying TCP/net connection.
type Conn struct {
	r      *bufio.Reader
	w      io.Writer
	closer io.Closer
	mu     sync.Mutex
	closed bool
}

// Upgrade upgrades an incoming HTTP request to a WebSocket connection.
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return nil, errors.New("websocket: method not allowed")
	}

	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "Missing Sec-WebSocket-Key header", http.StatusBadRequest)
		return nil, errors.New("websocket: missing key")
	}

	h := sha1.New()
	h.Write([]byte(key + websocketGUID))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Webserver doesn't support hijacking", http.StatusInternalServerError)
		return nil, errors.New("websocket: hijacking not supported")
	}

	netConn, buf, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("websocket hijack: %w", err)
	}

	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"

	if _, err := buf.WriteString(resp); err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("websocket handshake write: %w", err)
	}
	if err := buf.Flush(); err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("websocket handshake flush: %w", err)
	}

	return &Conn{
		r:      buf.Reader,
		w:      netConn,
		closer: netConn,
	}, nil
}

// ReadMessage reads the next frame from the connection, handling masking and frame headers.
func (c *Conn) ReadMessage() (int, []byte, error) {
	b0, err := c.r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	opcode := int(b0 & 0x0f)

	b1, err := c.r.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	isMasked := (b1 & 0x80) != 0
	payloadLen := int(b1 & 0x7f)

	if payloadLen == 126 {
		var extended uint16
		if err := binary.Read(c.r, binary.BigEndian, &extended); err != nil {
			return 0, nil, err
		}
		payloadLen = int(extended)
	} else if payloadLen == 127 {
		var extended uint64
		if err := binary.Read(c.r, binary.BigEndian, &extended); err != nil {
			return 0, nil, err
		}
		payloadLen = int(extended)
	}

	var maskKey [4]byte
	if isMasked {
		if _, err := io.ReadFull(c.r, maskKey[:]); err != nil {
			return 0, nil, err
		}
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(c.r, payload); err != nil {
		return 0, nil, err
	}

	if isMasked {
		for i := 0; i < payloadLen; i++ {
			payload[i] ^= maskKey[i%4]
		}
	}

	if opcode == OpClose {
		_ = c.Close()
		return OpClose, nil, io.EOF
	}

	if opcode == OpPing {
		_ = c.WriteMessage(OpPong, payload)
		return c.ReadMessage()
	}

	return opcode, payload, nil
}

// WriteMessage sends a WebSocket frame with unmasked payload (server-to-client).
func (c *Conn) WriteMessage(opcode int, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return net.ErrClosed
	}

	var header []byte
	b0 := byte(0x80 | (opcode & 0x0f)) // FIN bit set

	payloadLen := len(payload)
	if payloadLen < 126 {
		header = []byte{b0, byte(payloadLen)}
	} else if payloadLen <= 65535 {
		header = make([]byte, 4)
		header[0] = b0
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:], uint16(payloadLen))
	} else {
		header = make([]byte, 10)
		header[0] = b0
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:], uint64(payloadLen))
	}

	if _, err := c.w.Write(header); err != nil {
		return err
	}
	if len(payload) > 0 {
		if _, err := c.w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// WriteText sends a text message.
func (c *Conn) WriteText(text string) error {
	return c.WriteMessage(OpText, []byte(text))
}

// WriteJSON serializes and sends data as a text frame.
func (c *Conn) WriteJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.WriteMessage(OpText, data)
}

// Close closes the underlying network connection.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.closer != nil {
		return c.closer.Close()
	}
	return nil
}
