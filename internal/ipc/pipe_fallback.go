//go:build !windows

package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
)

var (
	fallbackPipeRegistryMu sync.RWMutex
	fallbackPipeRegistry   = make(map[string]*fallbackPipeHub)
)

type fallbackPipeHub struct {
	mu       sync.Mutex
	addr     string
	closed   bool
	connChan chan net.Conn
}

type fallbackPipeListener struct {
	hub *fallbackPipeHub
}

func (l *fallbackPipeListener) Accept() (net.Conn, error) {
	conn, ok := <-l.hub.connChan
	if !ok {
		return nil, errors.New("named pipe listener closed")
	}
	return conn, nil
}

func (l *fallbackPipeListener) Close() error {
	l.hub.mu.Lock()
	defer l.hub.mu.Unlock()

	if l.hub.closed {
		return nil
	}
	l.hub.closed = true

	fallbackPipeRegistryMu.Lock()
	delete(fallbackPipeRegistry, l.hub.addr)
	fallbackPipeRegistryMu.Unlock()

	close(l.hub.connChan)
	return nil
}

func (l *fallbackPipeListener) Addr() net.Addr {
	return fallbackPipeAddr(l.hub.addr)
}

type fallbackPipeAddr string

func (a fallbackPipeAddr) Network() string { return "pipe" }
func (a fallbackPipeAddr) String() string  { return string(a) }

func listenPipe(address string) (net.Listener, error) {
	if address == "" {
		address = DefaultPipeName
	}

	fallbackPipeRegistryMu.Lock()
	defer fallbackPipeRegistryMu.Unlock()

	if _, exists := fallbackPipeRegistry[address]; exists {
		return nil, fmt.Errorf("named pipe %q already in use", address)
	}

	hub := &fallbackPipeHub{
		addr:     address,
		connChan: make(chan net.Conn, 32),
	}
	fallbackPipeRegistry[address] = hub

	return &fallbackPipeListener{hub: hub}, nil
}

func dialPipe(ctx context.Context, address string) (net.Conn, error) {
	if address == "" {
		address = DefaultPipeName
	}

	fallbackPipeRegistryMu.RLock()
	hub, exists := fallbackPipeRegistry[address]
	fallbackPipeRegistryMu.RUnlock()

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
