package api

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/asr"
	"github.com/AdeshDeshmukh/cadence/internal/audio"
	"github.com/AdeshDeshmukh/cadence/internal/config"
	"github.com/AdeshDeshmukh/cadence/internal/storage"
	"github.com/AdeshDeshmukh/cadence/pkg/version"
)

// DaemonController defines the interface exposed by the daemon to the HTTP/WebSocket API.
type DaemonController interface {
	Status(ctx context.Context) (map[string]any, error)
	StartRecording(ctx context.Context) error
	StopRecording(ctx context.Context) (string, error)
	IsRecording() bool
	TranscribeAudio(ctx context.Context, chunks []audio.Chunk) (string, error)
}

// Server handles REST endpoints, SSE streams, and WebSocket connections.
type Server struct {
	cfg         config.HTTPConfig
	store       storage.Store
	controller  DaemonController
	broadcaster *Broadcaster
	asr         asr.Engine
	listener    net.Listener
	httpServer  *http.Server
	mu          sync.Mutex
	running     bool
}

// NewServer constructs an API Server.
func NewServer(
	cfg config.HTTPConfig,
	store storage.Store,
	controller DaemonController,
	asr asr.Engine,
) *Server {
	s := &Server{
		cfg:         cfg,
		store:       store,
		controller:  controller,
		broadcaster: NewBroadcaster(),
		asr:         asr,
	}
	return s
}

// Broadcaster returns the server's SSE event broadcaster.
func (s *Server) Broadcaster() *Broadcaster {
	return s.broadcaster
}

// Handler returns the HTTP handler routing all endpoints.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// REST Endpoints
	mux.HandleFunc("/v1/status", s.cors(s.auth(s.handleStatus)))
	mux.HandleFunc("/v1/history", s.cors(s.auth(s.handleHistory)))
	mux.HandleFunc("/v1/vocabulary", s.cors(s.auth(s.handleVocabulary)))
	mux.HandleFunc("/v1/snippets", s.cors(s.auth(s.handleSnippets)))
	mux.HandleFunc("/v1/notes", s.cors(s.auth(s.handleNotes)))
	mux.HandleFunc("/v1/notes/", s.cors(s.auth(s.handleNoteByID)))
	mux.HandleFunc("/v1/search", s.cors(s.auth(s.handleSearch)))
	mux.HandleFunc("/v1/transcribe", s.cors(s.auth(s.handleTranscribe)))
	mux.HandleFunc("/v1/dictate/start", s.cors(s.auth(s.handleDictateStart)))
	mux.HandleFunc("/v1/dictate/stop", s.cors(s.auth(s.handleDictateStop)))

	// SSE Real-time Events
	mux.HandleFunc("/v1/events", s.cors(s.auth(s.broadcaster.ServeHTTP)))

	// Streaming WebSocket
	mux.HandleFunc("/v1/ws", s.handleWebSocket)

	return mux
}

// Start launches the HTTP server listening on configured host and port.
func (s *Server) Start(l net.Listener) error {
	s.mu.Lock()
	s.listener = l
	s.httpServer = &http.Server{
		Handler: s.Handler(),
	}
	s.running = true
	s.mu.Unlock()

	return s.httpServer.Serve(l)
}

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.httpServer == nil {
		return nil
	}
	s.running = false
	return s.httpServer.Shutdown(ctx)
}

// Middleware: CORS
func (s *Server) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// Middleware: Bearer token auth if configured
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AuthToken != "" {
			authHeader := r.Header.Get("Authorization")
			expected := "Bearer " + s.cfg.AuthToken
			if authHeader != expected && r.URL.Query().Get("token") != s.cfg.AuthToken {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Server) writeError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]any{
		"ok":    false,
		"error": msg,
	})
}

// GET /v1/status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var statusMap map[string]any
	if s.controller != nil {
		sm, err := s.controller.Status(r.Context())
		if err == nil {
			statusMap = sm
		}
	}

	if statusMap == nil {
		statusMap = map[string]any{
			"status":    "running",
			"version":   version.Version,
			"recording": false,
		}
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"status": statusMap,
	})
}

// GET /v1/history?limit=20
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if s.store == nil {
		s.writeError(w, http.StatusInternalServerError, "store not configured")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	list, err := s.store.ListTranscripts(limit)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"transcripts": list,
		"count":       len(list),
	})
}

// GET /v1/vocabulary or POST /v1/vocabulary
func (s *Server) handleVocabulary(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		s.writeError(w, http.StatusInternalServerError, "store not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		filter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("filter")))
		entries, err := s.store.Vocabulary()
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if filter != "" {
			filtered := make([]storage.VocabularyEntry, 0, len(entries))
			for _, e := range entries {
				if strings.Contains(strings.ToLower(e.Spoken), filter) ||
					strings.Contains(strings.ToLower(e.Canonical), filter) {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}
		s.writeJSON(w, http.StatusOK, map[string]any{
			"ok":         true,
			"vocabulary": entries,
			"count":      len(entries),
		})

	case http.MethodPost:
		var req struct {
			Spoken    string `json:"spoken"`
			Canonical string `json:"canonical"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid request json")
			return
		}
		if req.Spoken == "" || req.Canonical == "" {
			s.writeError(w, http.StatusBadRequest, "spoken and canonical terms required")
			return
		}
		if err := s.store.SetVocabulary(storage.VocabularyEntry{
			Spoken:    req.Spoken,
			Canonical: req.Canonical,
		}); err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.broadcaster.Broadcast("vocabulary_updated", req)
		s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entry": req})

	default:
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// GET /v1/snippets
func (s *Server) handleSnippets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if s.store == nil {
		s.writeError(w, http.StatusInternalServerError, "store not configured")
		return
	}

	snippets, err := s.store.ListSnippets()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	if q != "" {
		filtered := make([]storage.Snippet, 0, len(snippets))
		for _, sn := range snippets {
			if strings.Contains(strings.ToLower(sn.Trigger), q) ||
				strings.Contains(strings.ToLower(sn.Description), q) ||
				strings.Contains(strings.ToLower(sn.Template), q) {
				filtered = append(filtered, sn)
			}
		}
		snippets = filtered
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"snippets": snippets,
		"count":    len(snippets),
	})
}

// GET /v1/notes
func (s *Server) handleNotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if s.store == nil {
		s.writeError(w, http.StatusInternalServerError, "store not configured")
		return
	}

	notes, err := s.store.ListNotes(storage.NoteFilter{})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"notes": notes,
		"count": len(notes),
	})
}

// GET /v1/notes/:id
func (s *Server) handleNoteByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/notes/")
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "missing note id")
		return
	}

	note, err := s.store.GetNote(id)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "note not found")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"note": note,
	})
}

// GET /v1/search?q=...&limit=10
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if s.store == nil {
		s.writeError(w, http.StatusInternalServerError, "store not configured")
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("query"))
	}
	if q == "" {
		s.writeError(w, http.StatusBadRequest, "missing search query parameter (q)")
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	results, err := s.store.Search(q, storage.SearchFilter{Limit: limit})
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"query":   q,
		"results": results,
		"count":   len(results),
	})
}

// POST /v1/transcribe (upload raw WAV or PCM)
func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "failed to read body: "+err.Error())
		return
	}
	defer r.Body.Close()

	if len(body) == 0 {
		s.writeError(w, http.StatusBadRequest, "empty audio payload")
		return
	}

	// Parse WAV or raw 16-bit PCM (16kHz mono)
	var samples []float32
	data := body
	// Check if body has RIFF/WAVE header
	if len(data) >= 44 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE" {
		// Find data chunk
		idx := 12
		for idx+8 <= len(data) {
			chunkID := string(data[idx : idx+4])
			chunkSize := int(binary.LittleEndian.Uint32(data[idx+4 : idx+8]))
			idx += 8
			if chunkID == "data" {
				if idx+chunkSize <= len(data) {
					data = data[idx : idx+chunkSize]
				} else {
					data = data[idx:]
				}
				break
			}
			idx += chunkSize
		}
	}

	samples = make([]float32, len(data)/2)
	for i := 0; i < len(samples); i++ {
		s16 := int16(binary.LittleEndian.Uint16(data[2*i : 2*i+2]))
		samples[i] = float32(s16) / 32768.0
	}
	chunks := []audio.Chunk{{Samples: samples, SampleRate: 16000, Timestamp: time.Now()}}

	var transcribedText string
	if s.controller != nil {
		text, err := s.controller.TranscribeAudio(r.Context(), chunks)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		transcribedText = text
	} else if s.asr != nil {
		res, err := s.asr.Transcribe(r.Context(), chunks)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		transcribedText = res.Text
	} else {
		s.writeError(w, http.StatusInternalServerError, "no speech recognizer available")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"text": transcribedText,
	})
}

// POST /v1/dictate/start
func (s *Server) handleDictateStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if s.controller == nil {
		s.writeError(w, http.StatusInternalServerError, "controller not configured")
		return
	}

	if err := s.controller.StartRecording(r.Context()); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.broadcaster.Broadcast("recording_started", map[string]any{"timestamp": time.Now().UTC()})
	s.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "recording"})
}

// POST /v1/dictate/stop
func (s *Server) handleDictateStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	if s.controller == nil {
		s.writeError(w, http.StatusInternalServerError, "controller not configured")
		return
	}

	text, err := s.controller.StopRecording(r.Context())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.broadcaster.Broadcast("recording_stopped", map[string]any{
		"text":      text,
		"timestamp": time.Now().UTC(),
	})
	s.writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"status": "stopped",
		"text":   text,
	})
}

// WebSocket message frames
type WSMessage struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload,omitempty"`
	Text    string         `json:"text,omitempty"`
}

// GET /v1/ws (WebSocket upgrade)
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := Upgrade(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	// Register SSE listener to relay server-side events over WebSocket too
	sseCh := s.broadcaster.Subscribe()
	defer s.broadcaster.Unsubscribe(sseCh)

	stopCh := make(chan struct{})
	defer close(stopCh)

	// Goroutine forwarding broadcast events to WebSocket client
	go func() {
		for {
			select {
			case <-stopCh:
				return
			case ev, ok := <-sseCh:
				if !ok {
					return
				}
				_ = conn.WriteJSON(map[string]any{
					"type":      "event",
					"event":     ev.Type,
					"data":      ev.Data,
					"timestamp": ev.Timestamp,
				})
			}
		}
	}()

	// Send initial greeting frame
	_ = conn.WriteJSON(map[string]any{
		"type":    "welcome",
		"version": version.Version,
		"status":  "connected",
	})

	// Message read loop
	for {
		op, data, err := conn.ReadMessage()
		if err != nil {
			break
		}

		switch op {
		case OpText:
			var msg WSMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				_ = conn.WriteJSON(map[string]any{"type": "error", "error": "invalid json"})
				continue
			}

			switch msg.Type {
			case "ping":
				_ = conn.WriteJSON(map[string]any{"type": "pong"})

			case "start_dictation":
				if s.controller != nil {
					_ = s.controller.StartRecording(r.Context())
					s.broadcaster.Broadcast("recording_started", map[string]any{"timestamp": time.Now().UTC()})
					_ = conn.WriteJSON(map[string]any{"type": "dictation_started"})
				}

			case "stop_dictation":
				if s.controller != nil {
					txt, _ := s.controller.StopRecording(r.Context())
					s.broadcaster.Broadcast("recording_stopped", map[string]any{
						"text":      txt,
						"timestamp": time.Now().UTC(),
					})
					_ = conn.WriteJSON(map[string]any{
						"type": "dictation_stopped",
						"text": txt,
					})
				}

			default:
				_ = conn.WriteJSON(map[string]any{
					"type":  "ack",
					"reply": msg.Type,
				})
			}

		case OpBinary:
			// Binary frames: streaming 16kHz 16-bit PCM mono audio chunk
			if len(data) >= 2 && s.asr != nil {
				samples := make([]float32, len(data)/2)
				for i := 0; i < len(samples); i++ {
					s16 := int16(data[2*i]) | (int16(data[2*i+1]) << 8)
					samples[i] = float32(s16) / 32768.0
				}
				chunk := []audio.Chunk{{Samples: samples, SampleRate: 16000, Timestamp: time.Now()}}
				if res, err := s.asr.Transcribe(r.Context(), chunk); err == nil && res.Text != "" {
					_ = conn.WriteJSON(map[string]any{
						"type":    "partial",
						"text":    res.Text,
						"partial": true,
					})
				}
			}
		}
	}
}
