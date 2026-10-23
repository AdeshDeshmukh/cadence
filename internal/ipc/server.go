package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
)

type Handler interface {
	Handle(context.Context, Request) (interface{}, error)
}

type Server struct {
	listener net.Listener
	handler  Handler
}

func NewServer(l net.Listener, h Handler) *Server {
	return &Server{listener: l, handler: h}
}

func (s *Server) Serve(ctx context.Context) error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go s.connection(ctx, conn)
	}
}

func (s *Server) connection(ctx context.Context, c net.Conn) {
	defer c.Close()

	scanner := bufio.NewScanner(c)
	encoder := json.NewEncoder(c)

	for scanner.Scan() {
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = encoder.Encode(Response{
				JSONRPC: "2.0",
				Error:   &Error{Code: -32700, Message: "invalid JSON"},
			})
			continue
		}

		result, err := s.handler.Handle(ctx, req)
		response := Response{
			JSONRPC: "2.0", ID: req.ID, Result: result,
		}

		if err != nil {
			response.Result = nil
			response.Error = &Error{Code: -32000, Message: err.Error()}
		}

		_ = encoder.Encode(response)
	}
}
