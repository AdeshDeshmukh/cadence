package edit

// Intent represents the classified category of a voice editing command.
type Intent string

const (
	// IntentNone indicates regular spoken dictation (not an editing command).
	IntentNone Intent = "none"

	// IntentRewrite requests rewriting, rephrasing, or fixing text.
	IntentRewrite Intent = "rewrite"

	// IntentShorten requests condensing or making text more concise.
	IntentShorten Intent = "shorten"

	// IntentExpand requests elaborating or expanding text with more detail.
	IntentExpand Intent = "expand"

	// IntentSummarize requests a summary or extraction of key points.
	IntentSummarize Intent = "summarize"

	// IntentTranslate requests translating text into another language.
	IntentTranslate Intent = "translate"

	// IntentFormatList requests structuring text into bulleted or numbered items.
	IntentFormatList Intent = "format_list"

	// IntentFormatTable requests structuring text into a tabular markdown format.
	IntentFormatTable Intent = "format_table"

	// IntentCustom represents an open-ended editing instruction.
	IntentCustom Intent = "custom"
)

// Instruction captures the parsed intent, target parameters, and metadata of an edit command.
type Instruction struct {
	Raw            string   `json:"raw"`
	Intent         Intent   `json:"intent"`
	IsEdit         bool     `json:"is_edit"`
	TargetLanguage string   `json:"target_language,omitempty"`
	Tone           string   `json:"tone,omitempty"`
	Format         string   `json:"format,omitempty"`
	Columns        []string `json:"columns,omitempty"`
	Prompt         string   `json:"prompt"`
	Confidence     float64  `json:"confidence"`
}
