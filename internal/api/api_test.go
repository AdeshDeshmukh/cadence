package api

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/config"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

// mockController implements DaemonController for API testing.
type mockController struct {
	mu        sync.Mutex
	recording bool
	status    map[string]any
	err       error
}

func (m *mockController) Status(ctx context.Context) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	res := map[string]any{
		"state":     "idle",
		"recording": m.recording,
	}
	for k, v := range m.status {
		res[k] = v
	}
	return res, nil
}

func (m *mockController) StartRecording(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.recording = true
	return nil
}

func (m *mockController) StopRecording(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return "", m.err
	}
	m.recording = false
	return "recorded transcribed text", nil
}

func (m *mockController) IsRecording() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.recording
}

func (m *mockController) TranscribeAudio(ctx context.Context, chunks []audio.Chunk) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return "", m.err
	}
	return "transcribed from chunks", nil
}

// mockASREngine implements asr.Engine for WebSocket / transcribe tests.
type mockASREngine struct{}

func (m *mockASREngine) Name() string { return "mock-asr" }
func (m *mockASREngine) Transcribe(ctx context.Context, chunks []audio.Chunk) (asr.Result, error) {
	return asr.Result{Text: "mock asr transcript", Confidence: 0.95}, nil
}
func (m *mockASREngine) Stream(ctx context.Context, ch <-chan audio.Chunk) (<-chan asr.PartialResult, error) {
	out := make(chan asr.PartialResult)
	close(out)
	return out, nil
}

func TestAPIRestEndpoints(t *testing.T) {
	store := storage.NewMemory()

	controller := &mockController{
		status: map[string]any{"model": "whisper-base"},
	}

	cfg := config.HTTPConfig{
		Enabled: true,
		Host:    "127.0.0.1",
		Port:    0,
	}

	server := NewServer(cfg, store, controller, &mockASREngine{})
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// 1. GET /v1/status
	t.Run("GET /v1/status", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/v1/status")
		if err != nil {
			t.Fatalf("GET /v1/status failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.StatusCode)
		}

		var payload struct {
			Ok     bool           `json:"ok"`
			Status map[string]any `json:"status"`
		}
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode status response: %v", err)
		}

		if !payload.Ok {
			t.Errorf("expected ok: true, got false")
		}
		if payload.Status["model"] != "whisper-base" {
			t.Errorf("expected model whisper-base, got %v", payload.Status["model"])
		}
	})

	// 2. Vocabulary GET and POST
	t.Run("Vocabulary GET and POST", func(t *testing.T) {
		// POST new vocabulary item
		vocPayload := `{"spoken": "k eight s", "canonical": "k8s"}`
		res, err := http.Post(ts.URL+"/v1/vocabulary", "application/json", strings.NewReader(vocPayload))
		if err != nil {
			t.Fatalf("POST /v1/vocabulary failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(res.Body)
			t.Fatalf("expected 200 OK, got %d: %s", res.StatusCode, string(body))
		}

		// GET vocabulary items
		getRes, err := http.Get(ts.URL + "/v1/vocabulary")
		if err != nil {
			t.Fatalf("GET /v1/vocabulary failed: %v", err)
		}
		defer getRes.Body.Close()

		if getRes.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", getRes.StatusCode)
		}

		var respData struct {
			Ok         bool                      `json:"ok"`
			Vocabulary []storage.VocabularyEntry `json:"vocabulary"`
			Count      int                       `json:"count"`
		}
		if err := json.NewDecoder(getRes.Body).Decode(&respData); err != nil {
			t.Fatalf("failed to decode vocabulary: %v", err)
		}

		found := false
		for _, w := range respData.Vocabulary {
			if w.Canonical == "k8s" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected to find 'k8s' in vocabulary list: %+v", respData.Vocabulary)
		}
	})

	// 3. Snippets GET
	t.Run("GET /v1/snippets", func(t *testing.T) {
		err := store.SetSnippet(storage.Snippet{
			Trigger:     "brb",
			Template:    "be right back",
			Description: "quick text",
		})
		if err != nil {
			t.Fatalf("failed to save snippet: %v", err)
		}

		res, err := http.Get(ts.URL + "/v1/snippets")
		if err != nil {
			t.Fatalf("GET /v1/snippets failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.StatusCode)
		}

		var respData struct {
			Ok       bool              `json:"ok"`
			Snippets []storage.Snippet `json:"snippets"`
		}
		if err := json.NewDecoder(res.Body).Decode(&respData); err != nil {
			t.Fatalf("failed to decode snippets: %v", err)
		}

		if len(respData.Snippets) == 0 || respData.Snippets[0].Trigger != "brb" {
			t.Errorf("expected snippet with trigger 'brb', got: %+v", respData.Snippets)
		}
	})

	// 4. Notes GET & GET by ID
	t.Run("Notes GET and GET by ID", func(t *testing.T) {
		note := storage.Note{
			ID:        "note-123",
			Title:     "Architecture Guide",
			Content:   "Microservices with Go",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := store.SaveNote(note); err != nil {
			t.Fatalf("failed to save note: %v", err)
		}

		// GET /v1/notes
		res, err := http.Get(ts.URL + "/v1/notes")
		if err != nil {
			t.Fatalf("GET /v1/notes failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.StatusCode)
		}

		var respData struct {
			Ok    bool           `json:"ok"`
			Notes []storage.Note `json:"notes"`
		}
		if err := json.NewDecoder(res.Body).Decode(&respData); err != nil {
			t.Fatalf("failed to decode notes: %v", err)
		}
		if len(respData.Notes) == 0 {
			t.Fatalf("expected at least 1 note, got %d", len(respData.Notes))
		}

		// GET /v1/notes/note-123
		resByID, err := http.Get(ts.URL + "/v1/notes/note-123")
		if err != nil {
			t.Fatalf("GET /v1/notes/note-123 failed: %v", err)
		}
		defer resByID.Body.Close()

		if resByID.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resByID.StatusCode)
		}

		var fetched struct {
			Ok   bool         `json:"ok"`
			Note storage.Note `json:"note"`
		}
		if err := json.NewDecoder(resByID.Body).Decode(&fetched); err != nil {
			t.Fatalf("failed to decode single note: %v", err)
		}
		if fetched.Note.ID != "note-123" || fetched.Note.Title != "Architecture Guide" {
			t.Errorf("unexpected note content: %+v", fetched.Note)
		}
	})

	// 5. History GET
	t.Run("GET /v1/history", func(t *testing.T) {
		entry := storage.Transcript{
			ID:        "hist-1",
			Text:      "Hello Cadence world",
			CreatedAt: time.Now(),
		}
		if err := store.SaveTranscript(entry); err != nil {
			t.Fatalf("failed to save transcript: %v", err)
		}

		res, err := http.Get(ts.URL + "/v1/history?limit=10")
		if err != nil {
			t.Fatalf("GET /v1/history failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.StatusCode)
		}

		var respData struct {
			Ok          bool                 `json:"ok"`
			Transcripts []storage.Transcript `json:"transcripts"`
		}
		if err := json.NewDecoder(res.Body).Decode(&respData); err != nil {
			t.Fatalf("failed to decode history: %v", err)
		}
		if len(respData.Transcripts) == 0 || respData.Transcripts[0].Text != "Hello Cadence world" {
			t.Errorf("unexpected history entries: %+v", respData.Transcripts)
		}
	})

	// 6. Search GET
	t.Run("GET /v1/search", func(t *testing.T) {
		res, err := http.Get(ts.URL + "/v1/search?q=Architecture")
		if err != nil {
			t.Fatalf("GET /v1/search failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.StatusCode)
		}

		var results map[string]any
		if err := json.NewDecoder(res.Body).Decode(&results); err != nil {
			t.Fatalf("failed to decode search results: %v", err)
		}
		if results["query"] != "Architecture" {
			t.Errorf("expected query Architecture, got %v", results["query"])
		}
	})

	// 7. Dictate Start & Stop
	t.Run("POST /v1/dictate/start & stop", func(t *testing.T) {
		startRes, err := http.Post(ts.URL+"/v1/dictate/start", "application/json", nil)
		if err != nil {
			t.Fatalf("POST /v1/dictate/start failed: %v", err)
		}
		defer startRes.Body.Close()

		if startRes.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", startRes.StatusCode)
		}
		if !controller.IsRecording() {
			t.Errorf("expected controller to be recording")
		}

		stopRes, err := http.Post(ts.URL+"/v1/dictate/stop", "application/json", nil)
		if err != nil {
			t.Fatalf("POST /v1/dictate/stop failed: %v", err)
		}
		defer stopRes.Body.Close()

		if stopRes.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", stopRes.StatusCode)
		}

		var stopPayload map[string]any
		if err := json.NewDecoder(stopRes.Body).Decode(&stopPayload); err != nil {
			t.Fatalf("failed to decode stop response: %v", err)
		}
		if stopPayload["text"] != "recorded transcribed text" {
			t.Errorf("unexpected transcribed text: %v", stopPayload["text"])
		}
		if controller.IsRecording() {
			t.Errorf("expected controller to have stopped recording")
		}
	})

	// 8. Transcribe Audio POST
	t.Run("POST /v1/transcribe", func(t *testing.T) {
		dummyAudio := make([]byte, 3200) // 100ms of 16kHz 16-bit PCM
		res, err := http.Post(ts.URL+"/v1/transcribe", "application/octet-stream", bytes.NewReader(dummyAudio))
		if err != nil {
			t.Fatalf("POST /v1/transcribe failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", res.StatusCode)
		}

		var payload map[string]any
		if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode transcribe response: %v", err)
		}
		if payload["text"] != "transcribed from chunks" {
			t.Errorf("unexpected transcribe response text: %v", payload["text"])
		}
	})
}

func TestAPIAuthenticationAndCORS(t *testing.T) {
	store := storage.NewMemory()

	cfg := config.HTTPConfig{
		Enabled:     true,
		Host:        "127.0.0.1",
		Port:        0,
		AuthToken:   "supersecrettoken",
		CORSOrigins: []string{"http://localhost:3000"},
	}

	server := NewServer(cfg, store, nil, nil)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// 1. Unauthenticated request should fail with 401
	res, err := http.Get(ts.URL + "/v1/status")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", res.StatusCode)
	}

	// 2. Authenticated request with Bearer token
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/status", nil)
	req.Header.Set("Authorization", "Bearer supersecrettoken")
	resAuth, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request with bearer token failed: %v", err)
	}
	resAuth.Body.Close()
	if resAuth.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK with bearer token, got %d", resAuth.StatusCode)
	}

	// 3. Pre-flight OPTIONS CORS check
	reqOpt, _ := http.NewRequest(http.MethodOptions, ts.URL+"/v1/status", nil)
	reqOpt.Header.Set("Origin", "http://localhost:3000")
	reqOpt.Header.Set("Access-Control-Request-Method", "GET")
	resOpt, err := http.DefaultClient.Do(reqOpt)
	if err != nil {
		t.Fatalf("preflight OPTIONS request failed: %v", err)
	}
	resOpt.Body.Close()
	if resOpt.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 No Content for CORS preflight, got %d", resOpt.StatusCode)
	}
	if allowedOrigin := resOpt.Header.Get("Access-Control-Allow-Origin"); allowedOrigin != "http://localhost:3000" {
		t.Errorf("expected CORS origin http://localhost:3000, got %s", allowedOrigin)
	}
}

func TestAPIServerSentEvents(t *testing.T) {
	store := storage.NewMemory()

	server := NewServer(config.HTTPConfig{Enabled: true}, store, nil, nil)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL+"/v1/events", nil)
	if err != nil {
		t.Fatalf("failed to create SSE request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("SSE request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for SSE, got %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Errorf("expected text/event-stream content type, got %s", resp.Header.Get("Content-Type"))
	}

	// Broadcast test event
	server.Broadcaster().Broadcast("partial", map[string]string{"text": "streamed partial text"})

	buf := make([]byte, 1024)
	n, err := resp.Body.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("failed reading SSE stream: %v", err)
	}

	content := string(buf[:n])
	if !strings.Contains(content, "event:") || !strings.Contains(content, "data:") {
		t.Errorf("expected SSE event framing, got: %s", content)
	}
	_ = resp.Body.Close()
	ts.CloseClientConnections()
}

func TestAPIWebSocket(t *testing.T) {
	store := storage.NewMemory()

	mockASR := &mockASREngine{}
	controller := &mockController{
		status: map[string]any{"model": "whisper-base"},
	}
	server := NewServer(config.HTTPConfig{Enabled: true}, store, controller, mockASR)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Dial WebSocket using raw TCP HTTP 101 Upgrade
	u := strings.TrimPrefix(ts.URL, "http://")
	conn, err := net.Dial("tcp", u)
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer conn.Close()

	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	secKey := base64.StdEncoding.EncodeToString(nonce)

	reqStr := fmt.Sprintf("GET /v1/ws HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n\r\n", u, secKey)

	if _, err := conn.Write([]byte(reqStr)); err != nil {
		t.Fatalf("failed to send handshake request: %v", err)
	}

	bufr := bufio.NewReader(conn)
	for {
		line, err := bufr.ReadString('\n')
		if err != nil {
			t.Fatalf("failed reading handshake response line: %v", err)
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	// Read initial welcome frame from server
	welcomeMsg, err := readWSClientFrame(bufr)
	if err != nil {
		t.Fatalf("failed to read welcome frame: %v", err)
	}
	if !strings.Contains(string(welcomeMsg), "connected") {
		t.Errorf("expected welcome message with 'connected', got: %s", string(welcomeMsg))
	}

	// Send Ping frame and expect Pong frame
	if err := writeWSClientFrame(conn, OpPing, []byte("hello-ping")); err != nil {
		t.Fatalf("failed to send ping frame: %v", err)
	}

	pongMsg, err := readWSClientFrame(bufr)
	if err != nil {
		t.Fatalf("failed to read pong frame: %v", err)
	}
	if string(pongMsg) != "hello-ping" {
		t.Errorf("expected pong payload 'hello-ping', got: %s", string(pongMsg))
	}

	// Send text JSON action: {"type": "start_dictation"}
	actionJSON := []byte(`{"type":"start_dictation"}`)
	if err := writeWSClientFrame(conn, OpText, actionJSON); err != nil {
		t.Fatalf("failed to send action frame: %v", err)
	}

	// Read response frames until dictation_started is received
	foundStarted := false
	for i := 0; i < 3; i++ {
		frame, err := readWSClientFrame(bufr)
		if err != nil {
			t.Fatalf("failed to read frame: %v", err)
		}
		if strings.Contains(string(frame), "dictation_started") {
			foundStarted = true
			break
		}
	}
	if !foundStarted {
		t.Errorf("expected dictation_started frame")
	}

	// Send binary audio PCM frame
	pcm := make([]byte, 1600) // 50ms of audio
	if err := writeWSClientFrame(conn, OpBinary, pcm); err != nil {
		t.Fatalf("failed to send binary pcm frame: %v", err)
	}

	// Read frames until transcription result frame is received
	foundTranscript := false
	for i := 0; i < 5; i++ {
		frame, err := readWSClientFrame(bufr)
		if err != nil {
			t.Fatalf("failed to read transcribe result frame: %v", err)
		}
		if strings.Contains(string(frame), "mock asr transcript") {
			foundTranscript = true
			break
		}
	}
	if !foundTranscript {
		t.Errorf("expected transcript response in result frame")
	}

	// Send close frame
	_ = writeWSClientFrame(conn, OpClose, nil)
	_ = conn.Close()
	ts.CloseClientConnections()
}

// Helpers for testing WebSocket client frames (masked as required by RFC 6455).
func writeWSClientFrame(w io.Writer, opcode byte, payload []byte) error {
	length := len(payload)
	header := []byte{0x80 | (opcode & 0x0f)}

	mask := []byte{0x12, 0x34, 0x56, 0x78}
	if length <= 125 {
		header = append(header, 0x80|byte(length))
	} else if length <= 65535 {
		header = append(header, 0x80|126, 0, 0)
		binary.BigEndian.PutUint16(header[2:4], uint16(length))
	} else {
		header = append(header, 0x80|127, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(header[2:10], uint64(length))
	}
	header = append(header, mask...)

	maskedPayload := make([]byte, length)
	for i := 0; i < length; i++ {
		maskedPayload[i] = payload[i] ^ mask[i%4]
	}

	if _, err := w.Write(header); err != nil {
		return err
	}
	if _, err := w.Write(maskedPayload); err != nil {
		return err
	}
	return nil
}

func readWSClientFrame(r io.Reader) ([]byte, error) {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	lenByte := buf[1] & 0x7f
	var length int64
	switch lenByte {
	case 126:
		ext := make([]byte, 2)
		if _, err := io.ReadFull(r, ext); err != nil {
			return nil, err
		}
		length = int64(binary.BigEndian.Uint16(ext))
	case 127:
		ext := make([]byte, 8)
		if _, err := io.ReadFull(r, ext); err != nil {
			return nil, err
		}
		length = int64(binary.BigEndian.Uint64(ext))
	default:
		length = int64(lenByte)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
