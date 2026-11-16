package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/AdeshDeshmukh/cadence/pkg/version"
)

// Server implements the Model Context Protocol (MCP) JSON-RPC 2.0 stdio server.
type Server struct {
	provider Provider
	in       io.Reader
	out      io.Writer
	mu       sync.Mutex
	tools    []Tool
	prompts  []Prompt
}

// NewServer constructs an MCP server configured with standard tools and resources.
func NewServer(p Provider, in io.Reader, out io.Writer) *Server {
	s := &Server{
		provider: p,
		in:       in,
		out:      out,
		tools:    defaultTools(),
		prompts:  defaultPrompts(),
	}
	return s
}

// defaultTools defines the official MCP tools exposed by Cadence.
func defaultTools() []Tool {
	return []Tool{
		{
			Name:        "cadence_get_history",
			Description: "Query recent voice dictation transcripts, meeting segments, or audio notes from Cadence.",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"limit": {
						Type:        "number",
						Description: "Maximum number of entries to retrieve (default: 20, max: 100)",
						Default:     20,
					},
					"type": {
						Type:        "string",
						Description: "Type of history: 'all', 'dictation', or 'meeting'",
						Enum:        []string{"all", "dictation", "meeting"},
					},
				},
			},
		},
		{
			Name:        "cadence_get_vocabulary",
			Description: "List custom technical and phonetic vocabulary terms configured in Cadence (spoken to canonical mapping).",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"filter": {
						Type:        "string",
						Description: "Optional keyword filter to match spoken or canonical terms",
					},
				},
			},
		},
		{
			Name:        "cadence_add_vocabulary",
			Description: "Add a new spoken-to-canonical vocabulary mapping in Cadence so voice recognition expands spoken phrases correctly.",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"spoken": {
						Type:        "string",
						Description: "The phonetic phrase spoken by the user (e.g. 'cube control')",
					},
					"canonical": {
						Type:        "string",
						Description: "The replacement canonical code or technical term (e.g. 'kubectl')",
					},
				},
				Required: []string{"spoken", "canonical"},
			},
		},
		{
			Name:        "cadence_get_snippets",
			Description: "Fetch stored text snippets, boilerplate templates, and trigger keywords from Cadence.",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"query": {
						Type:        "string",
						Description: "Optional keyword or trigger to filter snippets",
					},
				},
			},
		},
		{
			Name:        "cadence_get_context",
			Description: "Retrieve real-time developer context from Cadence: active application, active code editor (file, language, cursor line/column, selection, surrounding code), and terminal state.",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"include_editor": {
						Type:        "boolean",
						Description: "Whether to include active IDE/editor context (default: true)",
						Default:     true,
					},
					"include_terminal": {
						Type:        "boolean",
						Description: "Whether to include shell/terminal context (default: true)",
						Default:     true,
					},
				},
			},
		},
		{
			Name:        "cadence_search_knowledge",
			Description: "Perform hybrid full-text (FTS5) or semantic vector embedding search across Cadence notes, transcripts, and meeting summaries.",
			InputSchema: ToolInputSchema{
				Type: "object",
				Properties: map[string]ToolProperty{
					"query": {
						Type:        "string",
						Description: "Search query string or question",
					},
					"semantic": {
						Type:        "boolean",
						Description: "Enable semantic similarity vector embedding search (default: false)",
						Default:     false,
					},
					"limit": {
						Type:        "number",
						Description: "Maximum number of search results to return (default: 10)",
						Default:     10,
					},
				},
				Required: []string{"query"},
			},
		},
	}
}

// defaultPrompts defines standard MCP prompts supported by Cadence.
func defaultPrompts() []Prompt {
	return []Prompt{
		{
			Name:        "cadence_dictation_context",
			Description: "Injects recent user voice dictation and developer context into conversation.",
			Arguments: []PromptArgument{
				{
					Name:        "limit",
					Description: "Number of recent transcripts to include (default: 5)",
					Required:    false,
				},
			},
		},
		{
			Name:        "cadence_summarize_meeting",
			Description: "Generates an analysis prompt to extract key takeaways, decisions, and action items from recent meetings.",
			Arguments: []PromptArgument{
				{
					Name:        "meeting_id",
					Description: "Optional meeting ID to summarize",
					Required:    false,
				},
			},
		},
	}
}

// Serve reads JSON-RPC requests from standard input and writes responses until EOF or context cancellation.
func (s *Server) Serve(ctx context.Context) error {
	scanner := bufio.NewScanner(s.in)
	// Support large payloads (up to 4MB)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = s.sendError(nil, CodeParseError, "Parse error: invalid JSON", nil)
			continue
		}

		resp := s.handleRequest(ctx, req)
		// Notifications (id == nil) do not receive a response unless it was a call
		if req.ID != nil || resp.Error != nil {
			if err := s.send(resp); err != nil {
				return err
			}
		}
	}

	return scanner.Err()
}

func (s *Server) send(resp Response) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = s.out.Write(data)
	return err
}

func (s *Server) sendError(id any, code int, message string, data any) error {
	return s.send(Response{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error: &RPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	})
}

func (s *Server) handleRequest(ctx context.Context, req Request) Response {
	resp := Response{
		JSONRPC: JSONRPCVersion,
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		var params InitializeParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &params)
		}
		resp.Result = InitializeResult{
			ProtocolVersion: LatestMCPVersion,
			Capabilities: ServerCapabilities{
				Tools:     map[string]any{},
				Resources: map[string]any{},
				Prompts:   map[string]any{},
			},
			ServerInfo: ServerInfo{
				Name:    "cadence",
				Version: version.Version,
			},
		}
		return resp

	case "notifications/initialized":
		// Client acknowledgement, no result needed
		return resp

	case "ping":
		resp.Result = map[string]any{}
		return resp

	case "tools/list":
		resp.Result = map[string]any{
			"tools": s.tools,
		}
		return resp

	case "tools/call":
		var params ToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &RPCError{Code: CodeInvalidParams, Message: "Invalid tool call parameters"}
			return resp
		}
		result, err := s.executeTool(ctx, params)
		if err != nil {
			resp.Result = ToolCallResult{
				Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("Error: %v", err)}},
				IsError: true,
			}
			return resp
		}
		resp.Result = result
		return resp

	case "resources/list":
		resp.Result = map[string]any{
			"resources": s.listResources(),
		}
		return resp

	case "resources/read":
		var params ResourceReadParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &RPCError{Code: CodeInvalidParams, Message: "Invalid resource read parameters"}
			return resp
		}
		content, err := s.readResource(ctx, params.URI)
		if err != nil {
			resp.Error = &RPCError{Code: CodeInternalError, Message: err.Error()}
			return resp
		}
		resp.Result = ResourceReadResult{
			Contents: []ResourceContent{*content},
		}
		return resp

	case "prompts/list":
		resp.Result = map[string]any{
			"prompts": s.prompts,
		}
		return resp

	case "prompts/get":
		var params PromptGetParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &RPCError{Code: CodeInvalidParams, Message: "Invalid prompt get parameters"}
			return resp
		}
		result, err := s.getPrompt(ctx, params)
		if err != nil {
			resp.Error = &RPCError{Code: CodeInternalError, Message: err.Error()}
			return resp
		}
		resp.Result = result
		return resp

	default:
		resp.Error = &RPCError{
			Code:    CodeMethodNotFound,
			Message: fmt.Sprintf("Method not found: %s", req.Method),
		}
		return resp
	}
}

func (s *Server) executeTool(ctx context.Context, params ToolCallParams) (*ToolCallResult, error) {
	if s.provider == nil {
		return nil, errors.New("cadence provider not configured")
	}

	switch params.Name {
	case "cadence_get_history":
		limit := 20
		if l, ok := params.Arguments["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}
		kind, _ := params.Arguments["type"].(string)
		items, err := s.provider.GetHistory(ctx, limit, kind)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(items, "", "  ")
		return &ToolCallResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil

	case "cadence_get_vocabulary":
		filter, _ := params.Arguments["filter"].(string)
		vocab, err := s.provider.GetVocabulary(ctx, filter)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(vocab, "", "  ")
		return &ToolCallResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil

	case "cadence_add_vocabulary":
		spoken, _ := params.Arguments["spoken"].(string)
		canonical, _ := params.Arguments["canonical"].(string)
		if spoken == "" || canonical == "" {
			return nil, errors.New("both 'spoken' and 'canonical' arguments are required")
		}
		if err := s.provider.AddVocabulary(ctx, spoken, canonical); err != nil {
			return nil, err
		}
		return &ToolCallResult{
			Content: []ContentItem{{
				Type: "text",
				Text: fmt.Sprintf("Successfully added vocabulary mapping: %q -> %q", spoken, canonical),
			}},
		}, nil

	case "cadence_get_snippets":
		query, _ := params.Arguments["query"].(string)
		snippets, err := s.provider.GetSnippets(ctx, query)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(snippets, "", "  ")
		return &ToolCallResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil

	case "cadence_get_context":
		incEd := true
		if v, ok := params.Arguments["include_editor"].(bool); ok {
			incEd = v
		}
		incTerm := true
		if v, ok := params.Arguments["include_terminal"].(bool); ok {
			incTerm = v
		}
		ctxMap, err := s.provider.GetContext(ctx, incEd, incTerm)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(ctxMap, "", "  ")
		return &ToolCallResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil

	case "cadence_search_knowledge":
		query, _ := params.Arguments["query"].(string)
		if strings.TrimSpace(query) == "" {
			return nil, errors.New("'query' argument is required")
		}
		semantic, _ := params.Arguments["semantic"].(bool)
		limit := 10
		if l, ok := params.Arguments["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}
		hits, err := s.provider.SearchKnowledge(ctx, query, semantic, limit)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(hits, "", "  ")
		return &ToolCallResult{
			Content: []ContentItem{{Type: "text", Text: string(data)}},
		}, nil

	default:
		return nil, fmt.Errorf("unknown tool: %s", params.Name)
	}
}

func (s *Server) listResources() []Resource {
	return []Resource{
		{
			URI:         "cadence://transcripts/recent",
			Name:        "Recent Transcripts",
			Description: "Latest voice dictation transcripts",
			MimeType:    "application/json",
		},
		{
			URI:         "cadence://notes",
			Name:        "Knowledge Notes",
			Description: "All stored markdown notes and meeting syntheses",
			MimeType:    "application/json",
		},
		{
			URI:         "cadence://context/editor",
			Name:        "Editor Context",
			Description: "Active code editor, active file, and selection context",
			MimeType:    "application/json",
		},
		{
			URI:         "cadence://vocabulary",
			Name:        "Vocabulary",
			Description: "Configured spoken-to-canonical custom vocabulary",
			MimeType:    "application/json",
		},
		{
			URI:         "cadence://snippets",
			Name:        "Snippets",
			Description: "Saved text expansion snippets",
			MimeType:    "application/json",
		},
	}
}

func (s *Server) readResource(ctx context.Context, uri string) (*ResourceContent, error) {
	if s.provider == nil {
		return nil, errors.New("cadence provider not configured")
	}

	switch {
	case uri == "cadence://transcripts/recent":
		items, err := s.provider.GetHistory(ctx, 20, "all")
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(items, "", "  ")
		return &ResourceContent{URI: uri, MimeType: "application/json", Text: string(data)}, nil

	case uri == "cadence://notes":
		notes, err := s.provider.GetNotes(ctx, 50)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(notes, "", "  ")
		return &ResourceContent{URI: uri, MimeType: "application/json", Text: string(data)}, nil

	case strings.HasPrefix(uri, "cadence://notes/"):
		id := strings.TrimPrefix(uri, "cadence://notes/")
		note, err := s.provider.GetNote(ctx, id)
		if err != nil {
			return nil, err
		}
		return &ResourceContent{URI: uri, MimeType: "text/markdown", Text: note.Content}, nil

	case uri == "cadence://context/editor":
		ctxMap, err := s.provider.GetContext(ctx, true, false)
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(ctxMap, "", "  ")
		return &ResourceContent{URI: uri, MimeType: "application/json", Text: string(data)}, nil

	case uri == "cadence://vocabulary":
		vocab, err := s.provider.GetVocabulary(ctx, "")
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(vocab, "", "  ")
		return &ResourceContent{URI: uri, MimeType: "application/json", Text: string(data)}, nil

	case uri == "cadence://snippets":
		snippets, err := s.provider.GetSnippets(ctx, "")
		if err != nil {
			return nil, err
		}
		data, _ := json.MarshalIndent(snippets, "", "  ")
		return &ResourceContent{URI: uri, MimeType: "application/json", Text: string(data)}, nil

	default:
		return nil, fmt.Errorf("resource not found: %s", uri)
	}
}

func (s *Server) getPrompt(ctx context.Context, params PromptGetParams) (*PromptGetResult, error) {
	if s.provider == nil {
		return nil, errors.New("cadence provider not configured")
	}

	switch params.Name {
	case "cadence_dictation_context":
		limit := 5
		items, err := s.provider.GetHistory(ctx, limit, "all")
		if err != nil {
			return nil, err
		}
		ctxMap, _ := s.provider.GetContext(ctx, true, true)

		var sb strings.Builder
		sb.WriteString("User's recent voice dictations in Cadence:\n")
		for _, item := range items {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", item.CreatedAt.Format(time.Kitchen), item.Text))
		}
		if len(ctxMap) > 0 {
			ctxBytes, _ := json.MarshalIndent(ctxMap, "", "  ")
			sb.WriteString("\nCurrent developer workspace & editor context:\n")
			sb.WriteString(string(ctxBytes))
		}

		return &PromptGetResult{
			Description: "Active user dictations and code editor context",
			Messages: []PromptMessage{
				{
					Role:    "user",
					Content: ContentItem{Type: "text", Text: sb.String()},
				},
			},
		}, nil

	case "cadence_summarize_meeting":
		return &PromptGetResult{
			Description: "Meeting summary and action items analysis template",
			Messages: []PromptMessage{
				{
					Role: "user",
					Content: ContentItem{
						Type: "text",
						Text: "Please analyze the meeting transcript provided, structure the key discussion points, list confirmed decisions, and output assigned action items with owners and deadlines.",
					},
				},
			},
		}, nil

	default:
		return nil, fmt.Errorf("unknown prompt: %s", params.Name)
	}
}
