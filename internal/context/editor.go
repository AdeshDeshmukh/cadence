package appcontext

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SelectionRange represents line and column boundary coordinates of a selection.
type SelectionRange struct {
	StartLine int `json:"start_line"`
	StartCol  int `json:"start_col"`
	EndLine   int `json:"end_line"`
	EndCol    int `json:"end_col"`
}

// EditorContext captures rich context from a developer's code editor (VS Code, Cursor, etc.).
type EditorContext struct {
	EditorName         string          `json:"editor_name"`          // e.g. "vscode", "vscodium", "cursor"
	WorkspaceRoot      string          `json:"workspace_root"`       // project root directory
	FilePath           string          `json:"file_path"`            // full or relative path to open file
	FileName           string          `json:"file_name"`            // base name, e.g. "app.go"
	LanguageID         string          `json:"language_id"`          // "go", "typescript", "python", "rust", etc.
	Line               int             `json:"line"`                 // active cursor line (1-based)
	Column             int             `json:"column"`               // active cursor column (1-based)
	SelectionText      string          `json:"selection_text"`       // text currently highlighted in editor
	SelectionRange     *SelectionRange `json:"selection_range,omitempty"`
	SurroundingContext string          `json:"surrounding_context"`  // nearby code lines for LLM context
	Active             bool            `json:"active"`               // whether editor is currently active
	UpdatedAt          time.Time       `json:"updated_at"`
}

// EditorContextManager provides thread-safe access to the active code editor state.
type EditorContextManager struct {
	mu      sync.RWMutex
	current EditorContext
	ttl     time.Duration
}

// NewEditorContextManager creates a new EditorContextManager with a default 15-minute TTL.
func NewEditorContextManager() *EditorContextManager {
	return &EditorContextManager{
		ttl: 15 * time.Minute,
	}
}

// Set stores new editor context, auto-populating FileName if missing.
func (m *EditorContextManager) Set(ctx EditorContext) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctx.FileName == "" && ctx.FilePath != "" {
		ctx.FileName = filepath.Base(ctx.FilePath)
	}
	if ctx.LanguageID == "" && ctx.FilePath != "" {
		ctx.LanguageID = GuessLanguage(ctx.FilePath)
	}
	if ctx.UpdatedAt.IsZero() {
		ctx.UpdatedAt = time.Now()
	}
	ctx.Active = true
	m.current = ctx
}

// Get returns the current editor context if not expired or cleared.
func (m *EditorContextManager) Get() (EditorContext, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.current.Active {
		return EditorContext{}, false
	}
	if m.ttl > 0 && time.Since(m.current.UpdatedAt) > m.ttl {
		return EditorContext{}, false
	}
	return m.current, true
}

// Clear deactivates the stored editor context.
func (m *EditorContextManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.current = EditorContext{}
}

// IsActive returns whether a valid, non-expired editor context exists.
func (m *EditorContextManager) IsActive() bool {
	_, ok := m.Get()
	return ok
}

// FormatContextSummary generates a concise single-line or multi-line summary of editor state.
func (m *EditorContextManager) FormatContextSummary() string {
	ctx, ok := m.Get()
	if !ok {
		return "No active editor context"
	}

	var parts []string
	if ctx.EditorName != "" {
		parts = append(parts, fmt.Sprintf("Editor: %s", ctx.EditorName))
	}
	if ctx.FileName != "" {
		parts = append(parts, fmt.Sprintf("File: %s", ctx.FileName))
	}
	if ctx.LanguageID != "" {
		parts = append(parts, fmt.Sprintf("Lang: %s", ctx.LanguageID))
	}
	if ctx.Line > 0 {
		parts = append(parts, fmt.Sprintf("Line: %d:%d", ctx.Line, ctx.Column))
	}
	if ctx.SelectionText != "" {
		parts = append(parts, fmt.Sprintf("Selected: %d chars", len(ctx.SelectionText)))
	}
	return strings.Join(parts, " | ")
}

// FormatPromptContext formats the editor context into a system prompt injection for LLM edits or polishes.
func (m *EditorContextManager) FormatPromptContext() string {
	ctx, ok := m.Get()
	if !ok {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("[Code Editor Context]\n")
	if ctx.FilePath != "" {
		sb.WriteString(fmt.Sprintf("File: %s\n", ctx.FilePath))
	} else if ctx.FileName != "" {
		sb.WriteString(fmt.Sprintf("File: %s\n", ctx.FileName))
	}
	if ctx.LanguageID != "" {
		sb.WriteString(fmt.Sprintf("Language: %s\n", ctx.LanguageID))
	}
	if ctx.Line > 0 {
		sb.WriteString(fmt.Sprintf("Cursor: line %d, col %d\n", ctx.Line, ctx.Column))
	}
	if ctx.SurroundingContext != "" {
		sb.WriteString("Surrounding Code:\n```")
		if ctx.LanguageID != "" {
			sb.WriteString(ctx.LanguageID)
		}
		sb.WriteString("\n")
		sb.WriteString(strings.TrimSpace(ctx.SurroundingContext))
		sb.WriteString("\n```\n")
	}
	if ctx.SelectionText != "" {
		sb.WriteString("Selected Text:\n```")
		if ctx.LanguageID != "" {
			sb.WriteString(ctx.LanguageID)
		}
		sb.WriteString("\n")
		sb.WriteString(ctx.SelectionText)
		sb.WriteString("\n```\n")
	}
	return sb.String()
}

// GuessLanguage infers language ID from file extension.
func GuessLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return "go"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".js":
		return "javascript"
	case ".jsx":
		return "javascriptreact"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "cpp"
	case ".java":
		return "java"
	case ".kt", ".kts":
		return "kotlin"
	case ".swift":
		return "swift"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".sh", ".bash", ".zsh":
		return "shellscript"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".md", ".markdown":
		return "markdown"
	case ".html", ".htm":
		return "html"
	case ".css", ".scss", ".sass":
		return "css"
	case ".sql":
		return "sql"
	default:
		return ""
	}
}
