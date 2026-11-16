package mcp

import (
	"encoding/json"
)

// JSONRPCVersion is the standard JSON-RPC protocol version.
const JSONRPCVersion = "2.0"

// LatestMCPVersion is the Model Context Protocol specification version supported.
const LatestMCPVersion = "2024-11-05"

// Standard JSON-RPC 2.0 error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Request is a standard JSON-RPC 2.0 incoming frame.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a standard JSON-RPC 2.0 outgoing frame.
type Response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

// RPCError captures an error during JSON-RPC processing.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ClientInfo describes the connected MCP client (Claude Code, Cursor, Windsurf, etc.).
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeParams holds the client handshake parameters.
type InitializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      ClientInfo     `json:"clientInfo"`
}

// ServerInfo identifies Cadence to the MCP client.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ServerCapabilities defines features supported by Cadence MCP server.
type ServerCapabilities struct {
	Tools     map[string]any `json:"tools"`
	Resources map[string]any `json:"resources"`
	Prompts   map[string]any `json:"prompts"`
}

// InitializeResult is returned on successful MCP handshake.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

// ToolProperty defines JSON Schema property metadata for a tool argument.
type ToolProperty struct {
	Type        string        `json:"type"`
	Description string        `json:"description,omitempty"`
	Items       *ToolProperty `json:"items,omitempty"`
	Enum        []string      `json:"enum,omitempty"`
	Default     any           `json:"default,omitempty"`
}

// ToolInputSchema describes parameters accepted by a tool using JSON Schema.
type ToolInputSchema struct {
	Type       string                  `json:"type"`
	Properties map[string]ToolProperty `json:"properties"`
	Required   []string                `json:"required,omitempty"`
}

// Tool defines an invokable MCP tool.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema ToolInputSchema `json:"inputSchema"`
}

// ToolCallParams contains the tool invocation request.
type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// ContentItem holds a single text, image, or resource content block.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ToolCallResult is the outcome of executing an MCP tool.
type ToolCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// Resource represents a readable MCP resource entity.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceReadParams specifies the URI of a resource to read.
type ResourceReadParams struct {
	URI string `json:"uri"`
}

// ResourceContent contains the fetched content of a resource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// ResourceReadResult is returned from resources/read.
type ResourceReadResult struct {
	Contents []ResourceContent `json:"contents"`
}

// PromptArgument defines an input argument for an MCP prompt template.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Prompt describes a reusable prompt template.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptMessage represents a message turn inside a prompt.
type PromptMessage struct {
	Role    string      `json:"role"`
	Content ContentItem `json:"content"`
}

// PromptGetParams contains the prompt template invocation arguments.
type PromptGetParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments,omitempty"`
}

// PromptGetResult is returned from prompts/get.
type PromptGetResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}
