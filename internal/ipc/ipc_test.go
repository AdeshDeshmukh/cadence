package ipc_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/ipc"
)

type echoHandler struct{}

func (echoHandler) Handle(_ context.Context, req ipc.Request) (interface{}, error) {
	if req.Method == "ping" {
		return "pong", nil
	}
	return map[string]interface{}{"echo": req.Params}, nil
}

func TestIPCServerAndClient(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	server := ipc.NewServer(listener, echoHandler{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()

	client := ipc.Client{
		Network: "tcp",
		Address: listener.Addr().String(),
	}

	callCtx, callCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer callCancel()

	res, err := client.Call(callCtx, "ping", nil)
	if err != nil {
		t.Fatalf("call ping failed: %v", err)
	}
	if res != "pong" {
		t.Fatalf("expected 'pong', got %v", res)
	}
}
