package appcontext

import (
	"context"
	"strings"
)

// Category represents the high-level category of an application.
type Category string

const (
	CategoryGeneral  Category = "general"
	CategoryCode     Category = "code"
	CategoryChat     Category = "chat"
	CategoryTerminal Category = "terminal"
	CategoryBrowser  Category = "browser"
	CategoryDocument Category = "document"
	CategoryMedical  Category = "medical"
	CategoryLegal    Category = "legal"
)

// AppInfo holds metadata about the active foreground application and window.
type AppInfo struct {
	Name        string   `json:"name"`
	BundleID    string   `json:"bundle_id,omitempty"`
	WindowTitle string   `json:"window_title,omitempty"`
	ProcessID   int      `json:"pid,omitempty"`
	Category    Category `json:"category"`
}

// Detector defines the contract for querying the active desktop application.
type Detector interface {
	Detect(ctx context.Context) (AppInfo, error)
}

// Categorize determines the Category of an application based on its name, bundle ID, and window title.
func Categorize(name, bundleID, windowTitle string) Category {
	n := strings.ToLower(strings.TrimSpace(name))
	b := strings.ToLower(strings.TrimSpace(bundleID))
	w := strings.ToLower(strings.TrimSpace(windowTitle))

	// 1. Medical Clinical Systems (prioritized to prevent false keyword matches)
	medicalKeywords := []string{
		"epic", "hyperspace", "cerner", "athenahealth", "emr", "ehr",
		"clinical", "meditech", "allscripts", "eclinicalworks",
	}
	for _, kw := range medicalKeywords {
		if strings.Contains(n, kw) || strings.Contains(b, kw) || strings.Contains(w, kw) {
			return CategoryMedical
		}
	}

	// 2. Legal Systems
	legalKeywords := []string{
		"westlaw", "lexisnexis", "clio", "relativity", "docusign", "mycase",
	}
	for _, kw := range legalKeywords {
		if strings.Contains(n, kw) || strings.Contains(b, kw) || strings.Contains(w, kw) {
			return CategoryLegal
		}
	}

	// 3. Terminal Emulators (prioritized before code to handle com.googlecode.iterm2)
	termKeywords := []string{
		"terminal", "iterm", "iterm2", "alacritty", "kitty", "wezterm",
		"warp", "ghostty", "powershell", "cmd.exe", "bash", "zsh", "tmux",
	}
	for _, kw := range termKeywords {
		if strings.Contains(n, kw) || strings.Contains(b, kw) {
			return CategoryTerminal
		}
	}
	if n == "hyper" || strings.HasPrefix(n, "hyper ") || b == "co.zeit.hyper" {
		return CategoryTerminal
	}

	// 4. Code Editors & IDEs
	codeKeywords := []string{
		"vscode", "visual studio", "cursor", "sublime", "atom", "zed",
		"neovim", "vim", "emacs", "intellij", "pycharm", "webstorm", "goland",
		"xcode", "android studio", "clion", "rider", "eclipse", "nova",
	}
	for _, kw := range codeKeywords {
		if strings.Contains(n, kw) || strings.Contains(b, kw) {
			return CategoryCode
		}
	}
	if n == "code" || strings.HasPrefix(n, "code ") || strings.HasSuffix(n, " code") ||
		strings.Contains(n, "visual studio code") || b == "com.microsoft.vscode" || strings.Contains(b, ".vscode") {
		return CategoryCode
	}

	// 5. Chat & Messaging
	chatKeywords := []string{
		"slack", "discord", "teams", "telegram", "whatsapp", "signal",
		"messages", "wechat", "mattermost", "element", "zoom", "skype",
	}
	for _, kw := range chatKeywords {
		if strings.Contains(n, kw) || strings.Contains(b, kw) {
			return CategoryChat
		}
	}

	// 6. Web Browsers
	browserKeywords := []string{
		"chrome", "google chrome", "safari", "firefox", "brave", "edge",
		"arc", "opera", "vivaldi", "zen browser", "chromium",
	}
	for _, kw := range browserKeywords {
		if strings.Contains(n, kw) || strings.Contains(b, kw) {
			return CategoryBrowser
		}
	}

	// 7. Notes & Document Processors
	docKeywords := []string{
		"notion", "obsidian", "word", "pages", "google docs", "bear",
		"logseq", "evernote", "craft", "onenote", "scrivener", "libreoffice",
	}
	for _, kw := range docKeywords {
		if strings.Contains(n, kw) || strings.Contains(b, kw) {
			return CategoryDocument
		}
	}

	return CategoryGeneral
}
