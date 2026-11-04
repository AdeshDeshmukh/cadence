package polish

import (
	"errors"
	"strings"
	"sync"

	"github.com/AdeshDeshmukh/cadence/internal/storage"
)

// Profile represents a prompt persona and formatting rule set for LLM polishing.
type Profile struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	SystemPrompt string   `json:"system_prompt"`
	Temperature  float64  `json:"temperature"`
	CustomRules  []string `json:"custom_rules,omitempty"`
	IsBuiltin    bool     `json:"is_builtin"`
}

// Built-in standard profile names.
const (
	ProfileNameGeneral  = "general"
	ProfileNameCode     = "code"
	ProfileNameMedical  = "medical"
	ProfileNameLegal    = "legal"
	ProfileNameAcademic = "academic"
)

var builtinProfiles = map[string]Profile{
	ProfileNameGeneral: {
		Name:        ProfileNameGeneral,
		Description: "General natural prose with standard grammar, punctuation, and disfluency cleanup",
		SystemPrompt: `You are an expert real-time voice dictation cleanup engine.
Polish raw speech-to-text transcriptions into clear, natural prose while preserving the speaker's original meaning and intent.
Rules:
1. Fix grammar, spelling, typos, and punctuation.
2. Remove verbal disfluencies, filler words (um, uh, you know, like), stutters, and false starts.
3. Apply appropriate capitalization and paragraphs where natural breaks occur.
4. NEVER add conversational commentary, explanations, greetings, quotes, or prefaces.
5. Output ONLY the polished text.`,
		Temperature: 0.2,
		IsBuiltin:   true,
	},
	ProfileNameCode: {
		Name:        ProfileNameCode,
		Description: "Technical and programming dictation with identifier casing, code blocks, and syntax",
		SystemPrompt: `You are an expert technical coding dictation cleanup engine.
Your job is to format speech into code, markdown, terminal commands, and technical documentation.
Rules:
1. Convert spoken identifiers into appropriate casing (camelCase, snake_case, PascalCase, or kebab-case based on context).
2. Wrap inline code or identifiers in backticks (e.g. 'user id' -> ` + "`userID`" + `, 'error not nil' -> ` + "`err != nil`" + `).
3. Format multiline code blocks with appropriate markdown language tags (e.g. ` + "```go" + `).
4. Format shell commands, arguments, and flags accurately (e.g. ` + "`git commit -m \"...\"`" + `).
5. Never add commentary, explanations, or quotes. Output ONLY the code or technical prose.`,
		Temperature: 0.1,
		IsBuiltin:   true,
	},
	ProfileNameMedical: {
		Name:        ProfileNameMedical,
		Description: "Clinical documentation with anatomical, pharmaceutical, and SOAP formatting",
		SystemPrompt: `You are an expert medical transcriptionist and clinical documentation cleanup engine.
Polish clinical dictations accurately and concisely.
Rules:
1. Accurately spell anatomical, physiological, pathological, and pharmacological terms.
2. Format dosages, frequencies, vital signs, and metric units cleanly (e.g. 50 mg PO BID, BP 120/80 mmHg).
3. Structure standard clinical headings if spoken (Subjective, Objective, Assessment, Plan).
4. Do not alter clinical facts or diagnostic statements.
5. NEVER add conversational commentary. Output ONLY the clinical documentation.`,
		Temperature: 0.1,
		IsBuiltin:   true,
	},
	ProfileNameLegal: {
		Name:        ProfileNameLegal,
		Description: "Formal legal documentation, defined terms, citations, and contractual clauses",
		SystemPrompt: `You are an expert legal transcription cleanup engine.
Polish legal dictations into precise, formal legal prose.
Rules:
1. Retain legal terms of art, Latin phrases, and statutory or case citations accurately.
2. Maintain formal capitalization for defined terms (e.g. the Agreement, the Company, the Parties).
3. Use formal legal punctuation, section references (§, Paragraph), and clause structures.
4. Never add conversational filler, prefaces, or summaries. Output ONLY the formal legal text.`,
		Temperature: 0.1,
		IsBuiltin:   true,
	},
	ProfileNameAcademic: {
		Name:        ProfileNameAcademic,
		Description: "Scholarly prose, rigorous vocabulary, citations, and LaTeX math formatting",
		SystemPrompt: `You are an expert academic writing and transcription cleanup engine.
Polish scholarly dictations into rigorous, publication-grade academic prose.
Rules:
1. Use precise scholarly terminology and maintain an objective, academic tone.
2. Format mathematical and technical expressions using standard LaTeX notation (e.g. $E=mc^2$).
3. Preserve bibliographic citations, author-date parentheticals, and footnotes.
4. Ensure logical sentence transitions and clear paragraph structure.
5. Output ONLY the polished academic text.`,
		Temperature: 0.2,
		IsBuiltin:   true,
	},
}

// ProfileRegistry manages built-in and persistent user-defined polish profiles.
type ProfileRegistry struct {
	mu    sync.RWMutex
	store storage.Store
}

// NewProfileRegistry creates a profile registry backed optionally by a storage.Store.
func NewProfileRegistry(store storage.Store) *ProfileRegistry {
	return &ProfileRegistry{
		store: store,
	}
}

// Builtins returns a list of all predefined system profiles.
func Builtins() []Profile {
	return []Profile{
		builtinProfiles[ProfileNameGeneral],
		builtinProfiles[ProfileNameCode],
		builtinProfiles[ProfileNameMedical],
		builtinProfiles[ProfileNameLegal],
		builtinProfiles[ProfileNameAcademic],
	}
}

// Resolve looks up a profile by name. If not found or empty, returns ProfileNameGeneral.
func (r *ProfileRegistry) Resolve(name string) Profile {
	p, ok := r.Get(name)
	if ok {
		return p
	}
	return builtinProfiles[ProfileNameGeneral]
}

// Get finds a profile by name from store or built-ins.
func (r *ProfileRegistry) Get(name string) (Profile, bool) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return Profile{}, false
	}

	r.mu.RLock()
	store := r.store
	r.mu.RUnlock()

	// Check persistent custom profiles first
	if store != nil {
		if record, err := store.GetPolishProfile(name); err == nil && record != nil {
			return Profile{
				Name:         record.Name,
				Description:  record.Description,
				SystemPrompt: record.SystemPrompt,
				Temperature:  record.Temperature,
				CustomRules:  record.CustomRules,
				IsBuiltin:    false,
			}, true
		}
	}

	// Fallback to built-ins
	if builtin, ok := builtinProfiles[name]; ok {
		return builtin, true
	}

	return Profile{}, false
}

// List returns all available profiles (built-in and custom).
func (r *ProfileRegistry) List() []Profile {
	r.mu.RLock()
	store := r.store
	r.mu.RUnlock()

	profileMap := make(map[string]Profile)

	// Add built-ins first
	for k, v := range builtinProfiles {
		profileMap[k] = v
	}

	// Overlay custom profiles from database
	if store != nil {
		if list, err := store.ListPolishProfiles(); err == nil {
			for _, rec := range list {
				profileMap[rec.Name] = Profile{
					Name:         rec.Name,
					Description:  rec.Description,
					SystemPrompt: rec.SystemPrompt,
					Temperature:  rec.Temperature,
					CustomRules:  rec.CustomRules,
					IsBuiltin:    false,
				}
			}
		}
	}

	out := make([]Profile, 0, len(profileMap))
	// Always put built-ins in standard order first
	orderedKeys := []string{ProfileNameGeneral, ProfileNameCode, ProfileNameMedical, ProfileNameLegal, ProfileNameAcademic}
	seen := make(map[string]bool)
	for _, k := range orderedKeys {
		if p, ok := profileMap[k]; ok {
			out = append(out, p)
			seen[k] = true
		}
	}
	// Append any custom profiles
	for k, p := range profileMap {
		if !seen[k] {
			out = append(out, p)
		}
	}

	return out
}

// Set creates or updates a custom profile in the persistent store.
func (r *ProfileRegistry) Set(p Profile) error {
	name := strings.TrimSpace(strings.ToLower(p.Name))
	if name == "" {
		return errors.New("profile name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.store == nil {
		return errors.New("no persistent store configured for custom profiles")
	}

	temp := p.Temperature
	if temp <= 0 {
		temp = 0.2
	}

	sysPrompt := p.SystemPrompt
	if sysPrompt == "" {
		if b, ok := builtinProfiles[name]; ok {
			sysPrompt = b.SystemPrompt
		} else {
			sysPrompt = builtinProfiles[ProfileNameGeneral].SystemPrompt
		}
	}

	rec := storage.PolishProfile{
		Name:         name,
		Description:  p.Description,
		SystemPrompt: sysPrompt,
		Temperature:  temp,
		CustomRules:  p.CustomRules,
	}

	return r.store.SetPolishProfile(rec)
}

// Delete removes a custom profile from the persistent store.
func (r *ProfileRegistry) Delete(name string) error {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return errors.New("profile name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.store == nil {
		return errors.New("no persistent store configured")
	}

	return r.store.DeletePolishProfile(name)
}
