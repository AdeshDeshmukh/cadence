package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	appcontext "github.com/AdeshDeshmukh/cadence/internal/context"
	"github.com/AdeshDeshmukh/cadence/internal/mcp"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

type mockIPCClient struct {
	handlers map[string]func(params any) (any, error)
}

func (m *mockIPCClient) Call(ctx context.Context, method string, params any) (any, error) {
	if h, ok := m.handlers[method]; ok {
		return h(params)
	}
	return nil, nil
}

func setupTestServer(t *testing.T) (*mcp.Server, *storage.Memory, *bytes.Buffer, *io.PipeWriter) {
	t.Helper()
	store := storage.NewMemory()

	// Seed store
	_ = store.SaveTranscript(storage.Transcript{
		ID:        "tr-1",
		RawText:   "hello world",
		Text:      "Hello world.",
		Language:  "en",
		Mode:      "dictation",
		CreatedAt: time.Now().Add(-10 * time.Minute),
	})
	_ = store.SetVocabulary(storage.VocabularyEntry{
		Spoken:    "cube control",
		Canonical: "kubectl",
	})
	_ = store.SetSnippet(storage.Snippet{
		Trigger:     ";sig",
		Template:    "Best regards,\nAdesh",
		Description: "Email signature",
	})
	_ = store.SaveNote(storage.Note{
		ID:        "note-1",
		Title:     "Architecture Plan",
		Content:   "# Architecture\nCadence on-device engine.",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})

	edCtx := appcontext.NewEditorContextManager()
	edCtx.Set(appcontext.EditorContext{
		EditorName:    "vscode",
		FilePath:      "/workspace/main.go",
		LanguageID:    "go",
		Line:          42,
		Column:        10,
		SelectionText: "func main()",
		Active:        true,
	})

	prov := mcp.NewDirectProvider(store, edCtx, nil, nil, nil)

	pipeReader, pipeWriter := io.Pipe()
	outBuf := &bytes.Buffer{}

	server := mcp.NewServer(prov, pipeReader, outBuf)
	return server, store, outBuf, pipeWriter
}

func executeJSONRPC(t *testing.T, w *io.PipeWriter, out *bytes.Buffer, req mcp.Request) mcp.Response {
	t.Helper()
	out.Reset()

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	data = append(data, '\n')
	if _, err := w.Write(data); err != nil {
		t.Fatalf("write to pipe: %v", err)
	}

	// Give a few ms for the server loop to process
	time.Sleep(20 * time.Millisecond)

	line, err := out.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read response line: %v, raw output: %q", err, out.String())
	}

	var resp mcp.Response
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", string(line), err)
	}
	return resp
}

func TestMCP_InitializeAndPing(t *testing.T) {
	server, _, outBuf, pipeWriter := setupTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()
	defer pipeWriter.Close()

	// 1. Initialize
	initReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05","clientInfo":{"name":"test-client","version":"1.0"}}`),
	}
	resp := executeJSONRPC(t, pipeWriter, outBuf, initReq)
	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error)
	}
	resData, _ := json.Marshal(resp.Result)
	var initRes mcp.InitializeResult
	_ = json.Unmarshal(resData, &initRes)
	if initRes.ProtocolVersion != mcp.LatestMCPVersion || initRes.ServerInfo.Name != "cadence" {
		t.Fatalf("unexpected init response: %+v", initRes)
	}

	// 2. Ping
	pingReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      2,
		Method:  "ping",
	}
	pingResp := executeJSONRPC(t, pipeWriter, outBuf, pingReq)
	if pingResp.Error != nil {
		t.Fatalf("ping failed: %v", pingResp.Error)
	}
}

func TestMCP_ToolsListAndCalls(t *testing.T) {
	server, store, outBuf, pipeWriter := setupTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()
	defer pipeWriter.Close()

	// 1. tools/list
	listReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      10,
		Method:  "tools/list",
	}
	listResp := executeJSONRPC(t, pipeWriter, outBuf, listReq)
	if listResp.Error != nil {
		t.Fatalf("tools/list error: %v", listResp.Error)
	}
	listMap, ok := listResp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result for tools/list, got: %T", listResp.Result)
	}
	tools, ok := listMap["tools"].([]any)
	if !ok || len(tools) < 5 {
		t.Fatalf("expected at least 5 tools, got: %d", len(tools))
	}

	// 2. tools/call: cadence_get_history
	histReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      11,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"cadence_get_history","arguments":{"limit":5}}`),
	}
	histResp := executeJSONRPC(t, pipeWriter, outBuf, histReq)
	if histResp.Error != nil {
		t.Fatalf("history call failed: %v", histResp.Error)
	}
	histData, _ := json.Marshal(histResp.Result)
	var histRes mcp.ToolCallResult
	_ = json.Unmarshal(histData, &histRes)
	if len(histRes.Content) == 0 || !strings.Contains(histRes.Content[0].Text, "Hello world") {
		t.Fatalf("unexpected history content: %+v", histRes)
	}

	// 3. tools/call: cadence_get_vocabulary
	vocReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      12,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"cadence_get_vocabulary","arguments":{"filter":"cube"}}`),
	}
	vocResp := executeJSONRPC(t, pipeWriter, outBuf, vocReq)
	if vocResp.Error != nil {
		t.Fatalf("vocabulary call failed: %v", vocResp.Error)
	}
	vocData, _ := json.Marshal(vocResp.Result)
	var vocRes mcp.ToolCallResult
	_ = json.Unmarshal(vocData, &vocRes)
	if len(vocRes.Content) == 0 || !strings.Contains(vocRes.Content[0].Text, "kubectl") {
		t.Fatalf("unexpected vocab result: %+v", vocRes)
	}

	// 4. tools/call: cadence_add_vocabulary
	addReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      13,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"cadence_add_vocabulary","arguments":{"spoken":"gee tee","canonical":"git"}}`),
	}
	addResp := executeJSONRPC(t, pipeWriter, outBuf, addReq)
	if addResp.Error != nil {
		t.Fatalf("add vocabulary failed: %v", addResp.Error)
	}
	// Verify it was stored
	vocabEntries, _ := store.Vocabulary()
	found := false
	for _, v := range vocabEntries {
		if v.Canonical == "git" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected git to be added to vocabulary")
	}

	// 5. tools/call: cadence_get_snippets
	snipReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      14,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"cadence_get_snippets","arguments":{"query":"sig"}}`),
	}
	snipResp := executeJSONRPC(t, pipeWriter, outBuf, snipReq)
	if snipResp.Error != nil {
		t.Fatalf("snippets call failed: %v", snipResp.Error)
	}
	snipData, _ := json.Marshal(snipResp.Result)
	var snipRes mcp.ToolCallResult
	_ = json.Unmarshal(snipData, &snipRes)
	if len(snipRes.Content) == 0 || !strings.Contains(snipRes.Content[0].Text, "Best regards") {
		t.Fatalf("unexpected snippet content: %+v", snipRes)
	}

	// 6. tools/call: cadence_get_context
	ctxReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      15,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"cadence_get_context","arguments":{"include_editor":true}}`),
	}
	ctxResp := executeJSONRPC(t, pipeWriter, outBuf, ctxReq)
	if ctxResp.Error != nil {
		t.Fatalf("context call failed: %v", ctxResp.Error)
	}
	ctxData, _ := json.Marshal(ctxResp.Result)
	var ctxRes mcp.ToolCallResult
	_ = json.Unmarshal(ctxData, &ctxRes)
	if len(ctxRes.Content) == 0 || !strings.Contains(ctxRes.Content[0].Text, "func main()") {
		t.Fatalf("unexpected context content: %+v", ctxRes)
	}
}

func TestMCP_ResourcesAndPrompts(t *testing.T) {
	server, _, outBuf, pipeWriter := setupTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()
	defer pipeWriter.Close()

	// 1. resources/list
	resListReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      20,
		Method:  "resources/list",
	}
	resListResp := executeJSONRPC(t, pipeWriter, outBuf, resListReq)
	if resListResp.Error != nil {
		t.Fatalf("resources/list error: %v", resListResp.Error)
	}
	resMap, _ := resListResp.Result.(map[string]any)
	resources, _ := resMap["resources"].([]any)
	if len(resources) < 3 {
		t.Fatalf("expected at least 3 resources, got %d", len(resources))
	}

	// 2. resources/read: notes
	readReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      21,
		Method:  "resources/read",
		Params:  json.RawMessage(`{"uri":"cadence://notes"}`),
	}
	readResp := executeJSONRPC(t, pipeWriter, outBuf, readReq)
	if readResp.Error != nil {
		t.Fatalf("resources/read error: %v", readResp.Error)
	}
	readData, _ := json.Marshal(readResp.Result)
	var readRes mcp.ResourceReadResult
	_ = json.Unmarshal(readData, &readRes)
	if len(readRes.Contents) == 0 || !strings.Contains(readRes.Contents[0].Text, "Architecture Plan") {
		t.Fatalf("unexpected read result: %+v", readRes)
	}

	// 3. prompts/list
	pListReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      22,
		Method:  "prompts/list",
	}
	pListResp := executeJSONRPC(t, pipeWriter, outBuf, pListReq)
	if pListResp.Error != nil {
		t.Fatalf("prompts/list error: %v", pListResp.Error)
	}

	// 4. prompts/get: cadence_dictation_context
	pGetReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      23,
		Method:  "prompts/get",
		Params:  json.RawMessage(`{"name":"cadence_dictation_context"}`),
	}
	pGetResp := executeJSONRPC(t, pipeWriter, outBuf, pGetReq)
	if pGetResp.Error != nil {
		t.Fatalf("prompts/get error: %v", pGetResp.Error)
	}
	pGetData, _ := json.Marshal(pGetResp.Result)
	var pGetRes mcp.PromptGetResult
	_ = json.Unmarshal(pGetData, &pGetRes)
	if len(pGetRes.Messages) == 0 || !strings.Contains(pGetRes.Messages[0].Content.Text, "Hello world") {
		t.Fatalf("unexpected prompt get result: %+v", pGetRes)
	}
}

func TestMCP_ErrorHandling(t *testing.T) {
	server, _, outBuf, pipeWriter := setupTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Serve(ctx)
	}()
	defer pipeWriter.Close()

	// Unknown method
	unkReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      99,
		Method:  "unknown/method/xyz",
	}
	unkResp := executeJSONRPC(t, pipeWriter, outBuf, unkReq)
	if unkResp.Error == nil || unkResp.Error.Code != mcp.CodeMethodNotFound {
		t.Fatalf("expected MethodNotFound error, got: %+v", unkResp)
	}

	// Invalid tool
	badToolReq := mcp.Request{
		JSONRPC: mcp.JSONRPCVersion,
		ID:      100,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"nonexistent_tool"}`),
	}
	badToolResp := executeJSONRPC(t, pipeWriter, outBuf, badToolReq)
	toolRes, _ := json.Marshal(badToolResp.Result)
	var res mcp.ToolCallResult
	_ = json.Unmarshal(toolRes, &res)
	if !res.IsError {
		t.Fatalf("expected error result for nonexistent tool")
	}
}

func TestMCP_AutoAndIPCProvider(t *testing.T) {
	mockClient := &mockIPCClient{
		handlers: map[string]func(params any) (any, error){
			"ping": func(params any) (any, error) {
				return map[string]any{"ok": true}, nil
			},
			"history": func(params any) (any, error) {
				return []storage.Transcript{
					{ID: "ipc-1", Text: "Transcribed via IPC"},
				}, nil
			},
			"vocabulary": func(params any) (any, error) {
				return []storage.VocabularyEntry{
					{Spoken: "docker", Canonical: "Docker"},
				}, nil
			},
			"vocab_add": func(params any) (any, error) {
				return map[string]any{"ok": true}, nil
			},
			"snippet_list": func(params any) (any, error) {
				return []storage.Snippet{
					{Trigger: ";fn", Template: "func() {}"},
				}, nil
			},
			"active_app": func(params any) (any, error) {
				return map[string]any{"name": "VS Code"}, nil
			},
			"editor_context_get": func(params any) (any, error) {
				return map[string]any{"file_name": "app.go"}, nil
			},
			"search": func(params any) (any, error) {
				return map[string]any{
					"results": []storage.SearchResult{
						{ID: "res-1", Title: "Search Match"},
					},
				}, nil
			},
			"note_list": func(params any) (any, error) {
				return []storage.Note{
					{ID: "note-ipc", Title: "IPC Note"},
				}, nil
			},
			"note_get": func(params any) (any, error) {
				return storage.Note{ID: "note-ipc", Title: "IPC Note", Content: "Content"}, nil
			},
		},
	}

	ipcProv := mcp.NewIPCProvider(mockClient)
	autoProv := mcp.NewAutoProvider(ipcProv, nil)
	ctx := context.Background()

	// 1. History
	hist, err := autoProv.GetHistory(ctx, 10, "all")
	if err != nil || len(hist) != 1 || hist[0].Text != "Transcribed via IPC" {
		t.Fatalf("GetHistory via IPC failed: %v, got: %+v", err, hist)
	}

	// 2. Vocabulary
	voc, err := autoProv.GetVocabulary(ctx, "dock")
	if err != nil || len(voc) != 1 {
		t.Fatalf("GetVocabulary via IPC failed: %v", err)
	}

	// 3. Vocab add
	if err := autoProv.AddVocabulary(ctx, "k8s", "Kubernetes"); err != nil {
		t.Fatalf("AddVocabulary via IPC failed: %v", err)
	}

	// 4. Snippets
	snips, err := autoProv.GetSnippets(ctx, "fn")
	if err != nil || len(snips) != 1 {
		t.Fatalf("GetSnippets via IPC failed: %v", err)
	}

	// 5. Context
	cMap, err := autoProv.GetContext(ctx, true, true)
	if err != nil || cMap["editor"] == nil {
		t.Fatalf("GetContext via IPC failed: %v, got: %+v", err, cMap)
	}

	// 6. Search
	sHits, err := autoProv.SearchKnowledge(ctx, "query", false, 5)
	if err != nil || len(sHits) != 1 {
		t.Fatalf("SearchKnowledge via IPC failed: %v, got: %+v", err, sHits)
	}

	// 7. Notes
	notes, err := autoProv.GetNotes(ctx, 5)
	if err != nil || len(notes) != 1 {
		t.Fatalf("GetNotes via IPC failed: %v", err)
	}
	note, err := autoProv.GetNote(ctx, "note-ipc")
	if err != nil || note.Title != "IPC Note" {
		t.Fatalf("GetNote via IPC failed: %v", err)
	}
}
