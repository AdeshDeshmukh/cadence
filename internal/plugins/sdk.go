package plugins

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// Standard protocol message types.
const (
	TypeInit           = "init"
	TypeInitAck        = "init_ack"
	TypeTranscriptHook = "transcript_hook"
	TypeToolCall       = "tool_call"
	TypeCommand        = "command"
	TypePing           = "ping"
	TypePong           = "pong"
)

// Message is the standard JSON frame exchanged between Cadence and plugin processes over stdio or sockets.
type Message struct {
	ID      string         `json:"id,omitempty"`
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload,omitempty"`
	Error   string         `json:"error,omitempty"`
}

// TranscriptEvent encapsulates a completed transcription event sent to plugins with CapTranscriptionHook.
type TranscriptEvent struct {
	SessionID   string `json:"session_id"`
	Text        string `json:"text"`
	RawText     string `json:"raw_text"`
	Application string `json:"application,omitempty"`
}

// TranscriptResult is returned by the plugin, allowing modification or cancellation of pasting.
type TranscriptResult struct {
	ModifiedText string   `json:"modified_text,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	CancelPaste  bool     `json:"cancel_paste,omitempty"`
}

// ToolCallEvent represents an invocation of a plugin tool by Cadence's autonomous agent.
type ToolCallEvent struct {
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments"`
}

// ToolCallResult is the outcome of a plugin tool execution.
type ToolCallResult struct {
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// CommandEvent is dispatched when a user triggers a plugin command.
type CommandEvent struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// CommandResult represents the terminal or text output of a plugin command.
type CommandResult struct {
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Handler defines the contract implemented by a Cadence plugin.
type Handler interface {
	OnInit(ctx context.Context, config map[string]any) error
	OnTranscript(ctx context.Context, event TranscriptEvent) (*TranscriptResult, error)
	OnToolCall(ctx context.Context, event ToolCallEvent) (*ToolCallResult, error)
	OnCommand(ctx context.Context, event CommandEvent) (*CommandResult, error)
}

// BaseHandler provides a default no-op implementation of Handler.
type BaseHandler struct{}

func (b BaseHandler) OnInit(ctx context.Context, config map[string]any) error { return nil }
func (b BaseHandler) OnTranscript(ctx context.Context, event TranscriptEvent) (*TranscriptResult, error) {
	return &TranscriptResult{ModifiedText: event.Text}, nil
}
func (b BaseHandler) OnToolCall(ctx context.Context, event ToolCallEvent) (*ToolCallResult, error) {
	return nil, fmt.Errorf("tool %q not implemented", event.ToolName)
}
func (b BaseHandler) OnCommand(ctx context.Context, event CommandEvent) (*CommandResult, error) {
	return nil, fmt.Errorf("command %q not implemented", event.Command)
}

// PluginRunner runs the message loop for a plugin process over an input and output stream.
type PluginRunner struct {
	handler Handler
	in      io.Reader
	out     io.Writer
	mu      sync.Mutex
}

// NewPluginRunner constructs a runner wrapping the given Handler.
func NewPluginRunner(h Handler, in io.Reader, out io.Writer) *PluginRunner {
	return &PluginRunner{
		handler: h,
		in:      in,
		out:     out,
	}
}

// Run starts the processing loop and handles messages until EOF or context cancellation.
func (r *PluginRunner) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(r.in)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Message
		if err := json.Unmarshal(line, &req); err != nil {
			r.send(Message{Error: "malformed request json"})
			continue
		}

		resp := r.dispatch(ctx, req)
		if err := r.send(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (r *PluginRunner) send(m Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = r.out.Write(data)
	return err
}

func (r *PluginRunner) dispatch(ctx context.Context, req Message) Message {
	resp := Message{
		ID:   req.ID,
		Type: req.Type + "_resp",
	}

	switch req.Type {
	case TypePing:
		resp.Type = TypePong
		return resp

	case TypeInit:
		cfg, _ := req.Payload["config"].(map[string]any)
		if err := r.handler.OnInit(ctx, cfg); err != nil {
			resp.Error = err.Error()
		} else {
			resp.Type = TypeInitAck
			resp.Payload = map[string]any{"ok": true}
		}
		return resp

	case TypeTranscriptHook:
		var ev TranscriptEvent
		data, _ := json.Marshal(req.Payload)
		_ = json.Unmarshal(data, &ev)
		res, err := r.handler.OnTranscript(ctx, ev)
		if err != nil {
			resp.Error = err.Error()
		} else if res != nil {
			resp.Payload = map[string]any{
				"modified_text": res.ModifiedText,
				"cancel_paste":  res.CancelPaste,
				"tags":          res.Tags,
			}
		}
		return resp

	case TypeToolCall:
		var ev ToolCallEvent
		data, _ := json.Marshal(req.Payload)
		_ = json.Unmarshal(data, &ev)
		res, err := r.handler.OnToolCall(ctx, ev)
		if err != nil {
			resp.Error = err.Error()
		} else if res != nil {
			if res.Error != "" {
				resp.Error = res.Error
			}
			resp.Payload = map[string]any{"result": res.Result}
		}
		return resp

	case TypeCommand:
		var ev CommandEvent
		data, _ := json.Marshal(req.Payload)
		_ = json.Unmarshal(data, &ev)
		res, err := r.handler.OnCommand(ctx, ev)
		if err != nil {
			resp.Error = err.Error()
		} else if res != nil {
			if res.Error != "" {
				resp.Error = res.Error
			}
			resp.Payload = map[string]any{"output": res.Output}
		}
		return resp

	default:
		resp.Error = fmt.Sprintf("unsupported message type: %s", req.Type)
		return resp
	}
}

// HostClient allows Cadence to send calls and events to a running plugin subprocess.
type HostClient struct {
	in      *bufio.Scanner
	out     io.Writer
	mu      sync.Mutex
	msgID   uint64
	pending map[string]chan Message
}

// NewHostClient creates a host client for communicating with a plugin over standard I/O pipes.
func NewHostClient(r io.Reader, w io.Writer) *HostClient {
	hc := &HostClient{
		in:      bufio.NewScanner(r),
		out:     w,
		pending: make(map[string]chan Message),
	}
	return hc
}

// SendCall delivers a request and awaits a response.
func (c *HostClient) SendCall(ctx context.Context, req Message) (Message, error) {
	c.mu.Lock()
	c.msgID++
	if req.ID == "" {
		req.ID = fmt.Sprintf("msg-%d", c.msgID)
	}
	resCh := make(chan Message, 1)
	c.pending[req.ID] = resCh

	data, err := json.Marshal(req)
	if err != nil {
		c.mu.Unlock()
		return Message{}, err
	}
	data = append(data, '\n')
	_, err = c.out.Write(data)
	c.mu.Unlock()

	if err != nil {
		return Message{}, err
	}

	// Read lines until matching response
	for {
		select {
		case <-ctx.Done():
			return Message{}, ctx.Err()
		case resp := <-resCh:
			if resp.Error != "" {
				return resp, errors.New(resp.Error)
			}
			return resp, nil
		default:
			if !c.in.Scan() {
				if err := c.in.Err(); err != nil {
					return Message{}, err
				}
				return Message{}, io.EOF
			}
			var resp Message
			if err := json.Unmarshal(c.in.Bytes(), &resp); err == nil {
				c.mu.Lock()
				if ch, ok := c.pending[resp.ID]; ok {
					ch <- resp
					delete(c.pending, resp.ID)
				}
				c.mu.Unlock()
			}
		}
	}
}

// Ping checks if the plugin is responsive.
func (c *HostClient) Ping(ctx context.Context) error {
	ctxTimeout, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := c.SendCall(ctxTimeout, Message{Type: TypePing})
	return err
}
