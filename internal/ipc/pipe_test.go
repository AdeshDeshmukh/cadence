package ipc

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

type echoHandler struct{}

func (e echoHandler) Handle(ctx context.Context, req Request) (interface{}, error) {
	return map[string]interface{}{
		"echo":   req.Method,
		"params": req.Params,
	}, nil
}

func TestDefaultSocketPath(t *testing.T) {
	path := DefaultSocketPath()
	if runtime.GOOS == "windows" {
		if path != DefaultPipeName {
			t.Errorf("expected %s on Windows, got %s", DefaultPipeName, path)
		}
	} else {
		if !strings.Contains(path, "cadence.sock") {
			t.Errorf("expected unix socket path, got %s", path)
		}
	}
}

func TestIsNamedPipe(t *testing.T) {
	if !IsNamedPipe(`\\.\pipe\cadence`) {
		t.Errorf("expected true for \\\\.\\pipe\\cadence")
	}
	if IsNamedPipe(`/tmp/cadence.sock`) {
		t.Errorf("expected false for /tmp/cadence.sock")
	}
	if IsNamedPipe(`127.0.0.1:8080`) {
		t.Errorf("expected false for 127.0.0.1:8080")
	}
}

func TestNamedPipe_ServerClientRoundTrip(t *testing.T) {
	pipeAddr := `\\.\pipe\cadence-test-roundtrip`

	l, err := Listen("pipe", pipeAddr)
	if err != nil {
		t.Fatalf("Listen on pipe failed: %v", err)
	}
	defer l.Close()

	if l.Addr().Network() != "pipe" {
		t.Errorf("expected network 'pipe', got %s", l.Addr().Network())
	}

	// Double listen should fail
	if _, err := Listen("pipe", pipeAddr); err == nil {
		t.Errorf("expected error on duplicate pipe listen")
	}

	server := NewServer(l, echoHandler{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()

	client := Client{
		Network: "pipe",
		Address: pipeAddr,
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()

	result, err := client.Call(callCtx, "ping", "hello-pipe")
	if err != nil {
		t.Fatalf("client.Call over named pipe failed: %v", err)
	}

	resMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}

	if resMap["echo"] != "ping" || resMap["params"] != "hello-pipe" {
		t.Fatalf("unexpected echo response: %+v", resMap)
	}
}

func TestNamedPipe_DialNonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := Dial(ctx, "pipe", `\\.\pipe\non-existent-pipe`)
	if err == nil {
		t.Fatalf("expected error dialing non-existent named pipe")
	}
}
