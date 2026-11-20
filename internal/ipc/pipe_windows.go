//go:build windows

package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
)

var (
	winPipeRegistryMu sync.RWMutex
	winPipeRegistry   = make(map[string]*winPipeHub)
)

type winPipeHub struct {
	mu       sync.Mutex
	addr     string
	closed   bool
	connChan chan net.Conn
}

type winPipeListener struct {
	hub *winPipeHub
}

func (l *winPipeListener) Accept() (net.Conn, error) {
	conn, ok := <-l.hub.connChan
	if !ok {
		return nil, errors.New("named pipe listener closed")
	}
	return conn, nil
}

func (l *winPipeListener) Close() error {
	l.hub.mu.Lock()
	defer l.hub.mu.Unlock()

	if l.hub.closed {
		return nil
	}
	l.hub.closed = true

	winPipeRegistryMu.Lock()
	delete(winPipeRegistry, l.hub.addr)
	winPipeRegistryMu.Unlock()

	close(l.hub.connChan)
	return nil
}

func (l *winPipeListener) Addr() net.Addr {
	return winPipeAddr(l.hub.addr)
}

type winPipeAddr string

func (a winPipeAddr) Network() string { return "pipe" }
func (a winPipeAddr) String() string  { return string(a) }

func listenPipe(address string) (net.Listener, error) {
	if address == "" {
		address = DefaultPipeName
	}

	winPipeRegistryMu.Lock()
	defer winPipeRegistryMu.Unlock()

	if _, exists := winPipeRegistry[address]; exists {
		return nil, fmt.Errorf("named pipe %q already in use", address)
	}

	hub := &winPipeHub{
		addr:     address,
		connChan: make(chan net.Conn, 32),
	}
	winPipeRegistry[address] = hub

	return &winPipeListener{hub: hub}, nil
}

func dialPipe(ctx context.Context, address string) (net.Conn, error) {
	if address == "" {
		address = DefaultPipeName
	}

	winPipeRegistryMu.RLock()
	hub, exists := winPipeRegistry[address]
	winPipeRegistryMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("named pipe %q not found or server offline", address)
	}

	hub.mu.Lock()
	if hub.closed {
		hub.mu.Unlock()
		return nil, errors.New("named pipe server closed")
	}
	hub.mu.Unlock()

	clientConn, serverConn := net.Pipe()

	select {
	case <-ctx.Done():
		_ = clientConn.Close()
		_ = serverConn.Close()
		return nil, ctx.Err()
	case hub.connChan <- serverConn:
		return clientConn, nil
	}
}
