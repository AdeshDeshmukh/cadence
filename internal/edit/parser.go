package edit

import (
	"context"
	"regexp"
	"strings"

	"github.com/AdeshDeshmukh/cadence/internal/polish"
)

// Parser parses natural voice instructions into structured edit commands.
type Parser struct {
	polisher polish.Polisher
}

// NewParser creates a new voice instruction parser.
func NewParser(polisher polish.Polisher) *Parser {
	return &Parser{
		polisher: polisher,
	}
}

var (
	// Format table patterns
	reFormatTable = regexp.MustCompile(`(?i)^(?:please\s+)?(?:(?:convert|format|turn|make)(?:\s+this)?\s+(?:(?:in)?to|as)?\s*(?:a\s+)?(?:markdown\s+)?table|tabularize(?:\s+this)?)\s*(.*)$`)
	reColumns     = regexp.MustCompile(`(?i)(?:with\s+)?(?:columns?|headers?)\s+(.*)$`)

	// Format list patterns
	reFormatList = regexp.MustCompile(`(?i)^(?:please\s+)?(?:convert|format|turn|make|bulletize)(?:\s+this)?\s+(?:(?:in)?to|as)?\s*(?:a\s+)?(bullet(?:ed)?\s+list|numbered\s+list|checklist|check\s+list|todo\s+list|task\s+list|bullets?|list)\s*(.*)$`)

	// Translate patterns
	reTranslate = regexp.MustCompile(`(?i)^(?:please\s+)?(?:translate(?:\s+this)?\s+(?:in)?to|say(?:\s+this)?\s+in|convert(?:\s+this)?\s+(?:in)?to)\s+(?:the\s+)?([a-zA-Z]+)(?:\s+(?:language|please))?\s*(.*)$`)

	// Summarize patterns
	reSummarize = regexp.MustCompile(`(?i)^(?:please\s+)?(?:summarize(?:\s+this)?|give\s+me\s+a\s+summary(?:\s+of\s+this)?|tldr(?:\s+this)?|key\s+(?:points|takeaways)(?:\s+from\s+this)?)\s*(.*)$`)

	// Shorten patterns
	reShorten = regexp.MustCompile(`(?i)^(?:please\s+)?(?:make(?:\s+this)?\s+(?:shorter|more\s+concise|brief)|condense(?:\s+this)?|shorten(?:\s+this)?|cut(?:\s+this)?\s+down)\s*(.*)$`)

	// Expand patterns
	reExpand = regexp.MustCompile(`(?i)^(?:please\s+)?(?:make(?:\s+this)?\s+(?:longer|more\s+detailed|elaborate)|expand(?:\s+on)?(?:\s+this)?|add(?:\s+more)?\s+details?(?:\s+to\s+this)?|elaborate(?:\s+on\s+this)?)\s*(.*)$`)

	// Rewrite with tone patterns
	reRewriteTone = regexp.MustCompile(`(?i)^(?:please\s+)?(?:make(?:\s+this)?(?:\s+sound)?|rewrite(?:\s+this)?(?:\s+to\s+(?:be|sound))?|change(?:\s+the)?\s+tone\s+to)\s+(professional|casual|polite|friendly|formal|informal|academic|executive|persuasive)\b\s*(.*)$`)

	// General rewrite / grammar fix patterns
	reRewrite = regexp.MustCompile(`(?i)^(?:please\s+)?(?:rewrite(?:\s+this)?|paraphrase(?:\s+this)?|rephrase(?:\s+this)?|fix(?:\s+(?:the|all))?\s*(?:grammar|spelling|typos|errors)(?:\s+in\s+this)?)\s*(.*)$`)

	// Open-ended custom edit triggers
	reCustomEdit = regexp.MustCompile(`(?i)^(?:please\s+)?(?:edit|modify|update|change|replace|format)\s+(.*)$`)
)

func capitalize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

// Parse analyzes a spoken phrase and extracts the intended edit instruction.
func (p *Parser) Parse(ctx context.Context, input string) Instruction {
	clean := strings.TrimSpace(input)
	clean = strings.TrimRight(clean, ".!?")
	clean = strings.TrimSpace(clean)

	if clean == "" {
		return Instruction{
			Raw:        input,
			Intent:     IntentNone,
			IsEdit:     false,
			Confidence: 0.0,
		}
	}

	// 1. Format as Table
	if m := reFormatTable.FindStringSubmatch(clean); len(m) > 0 {
		extra := ""
		if len(m) > 1 {
			extra = strings.TrimSpace(m[1])
		}
		var cols []string
		if cm := reColumns.FindStringSubmatch(extra); len(cm) > 1 {
			rawCols := reItemSplit.Split(cm[1], -1)
			for _, c := range rawCols {
				cl := strings.TrimSpace(c)
				if cl != "" {
					cols = append(cols, cl)
				}
			}
		}
		return Instruction{
			Raw:        input,
			Intent:     IntentFormatTable,
			IsEdit:     true,
			Format:     "table",
			Columns:    cols,
			Prompt:     extra,
			Confidence: 0.95,
		}
	}

	// 2. Format as List
	if m := reFormatList.FindStringSubmatch(clean); len(m) > 1 {
		listType := "bullet_list"
		lowerMatch := strings.ToLower(m[1])
		if strings.Contains(lowerMatch, "numbered") {
			listType = "numbered_list"
		} else if strings.Contains(lowerMatch, "check") || strings.Contains(lowerMatch, "todo") || strings.Contains(lowerMatch, "task") {
			listType = "checklist"
		}
		extra := ""
		if len(m) > 2 {
			extra = strings.TrimSpace(m[2])
		}
		return Instruction{
			Raw:        input,
			Intent:     IntentFormatList,
			IsEdit:     true,
			Format:     listType,
			Prompt:     extra,
			Confidence: 0.95,
		}
	}

	// 3. Translate
	if m := reTranslate.FindStringSubmatch(clean); len(m) > 1 {
		lang := capitalize(m[1])
		lower := strings.ToLower(lang)
		if lower != "a" && lower != "an" && lower != "the" && lower != "table" && lower != "list" {
			extra := ""
			if len(m) > 2 {
				extra = strings.TrimSpace(m[2])
			}
			return Instruction{
				Raw:            input,
				Intent:         IntentTranslate,
				IsEdit:         true,
				TargetLanguage: lang,
				Prompt:         extra,
				Confidence:     0.98,
			}
		}
	}

	// 4. Summarize
	if m := reSummarize.FindStringSubmatch(clean); len(m) > 0 {
		extra := ""
		if len(m) > 1 {
			extra = strings.TrimSpace(m[1])
		}
		format := ""
		if strings.Contains(strings.ToLower(clean), "bullet") {
			format = "bullet_list"
		}
		return Instruction{
			Raw:        input,
			Intent:     IntentSummarize,
			IsEdit:     true,
			Format:     format,
			Prompt:     extra,
			Confidence: 0.95,
		}
	}

	// 5. Shorten
	if m := reShorten.FindStringSubmatch(clean); len(m) > 0 {
		extra := ""
		if len(m) > 1 {
			extra = strings.TrimSpace(m[1])
		}
		return Instruction{
			Raw:        input,
			Intent:     IntentShorten,
			IsEdit:     true,
			Tone:       "concise",
			Prompt:     extra,
			Confidence: 0.95,
		}
	}

	// 6. Expand
	if m := reExpand.FindStringSubmatch(clean); len(m) > 0 {
		extra := ""
		if len(m) > 1 {
			extra = strings.TrimSpace(m[1])
		}
		return Instruction{
			Raw:        input,
			Intent:     IntentExpand,
			IsEdit:     true,
			Prompt:     extra,
			Confidence: 0.95,
		}
	}

	// 7. Rewrite with explicit tone
	if m := reRewriteTone.FindStringSubmatch(clean); len(m) > 1 {
		tone := strings.ToLower(strings.TrimSpace(m[1]))
		extra := ""
		if len(m) > 2 {
			extra = strings.TrimSpace(m[2])
		}
		return Instruction{
			Raw:        input,
			Intent:     IntentRewrite,
			IsEdit:     true,
			Tone:       tone,
			Prompt:     extra,
			Confidence: 0.95,
		}
	}

	// 8. General Rewrite / Grammar correction
	if m := reRewrite.FindStringSubmatch(clean); len(m) > 0 {
		extra := ""
		if len(m) > 1 {
			extra = strings.TrimSpace(m[1])
		}
		return Instruction{
			Raw:        input,
			Intent:     IntentRewrite,
			IsEdit:     true,
			Prompt:     extra,
			Confidence: 0.90,
		}
	}

	// 9. Open-ended custom edit command
	if m := reCustomEdit.FindStringSubmatch(clean); len(m) > 0 {
		return Instruction{
			Raw:        input,
			Intent:     IntentCustom,
			IsEdit:     true,
			Prompt:     clean,
			Confidence: 0.85,
		}
	}

	// 10. Default: Regular voice dictation (not an edit command)
	return Instruction{
		Raw:        input,
		Intent:     IntentNone,
		IsEdit:     false,
		Confidence: 0.0,
	}
}
