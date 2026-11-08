package developer

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// CasingStyle represents the casing convention to apply to words.
type CasingStyle string

const (
	CaseAuto     CasingStyle = "auto"
	CaseSnake    CasingStyle = "snake"
	CaseCamel    CasingStyle = "camel"
	CasePascal   CasingStyle = "pascal"
	CaseKebab    CasingStyle = "kebab"
	CaseConstant CasingStyle = "constant"
	CaseDot      CasingStyle = "dot"
)

// CodeFormatRequest defines the input for code-mode formatting.
type CodeFormatRequest struct {
	Text             string      `json:"text"`
	Style            CasingStyle `json:"style,omitempty"`
	Language         string      `json:"language,omitempty"`
	IsDocstring      bool        `json:"is_docstring,omitempty"`
	PreserveAcronyms bool        `json:"preserve_acronyms"`
}

// CodeFormatResult holds the outcome of the code formatting transformation.
type CodeFormatResult struct {
	Original string      `json:"original"`
	Result   string      `json:"result"`
	Style    CasingStyle `json:"style"`
	Matched  bool        `json:"matched"`
}

var commonInitialisms = map[string]string{
	"id":    "ID",
	"url":   "URL",
	"http":  "HTTP",
	"https": "HTTPS",
	"api":   "API",
	"json":  "JSON",
	"db":    "DB",
	"uuid":  "UUID",
	"ip":    "IP",
	"sql":   "SQL",
	"jwt":   "JWT",
	"html":  "HTML",
	"css":   "CSS",
	"ssh":   "SSH",
	"tcp":   "TCP",
	"udp":   "UDP",
	"rpc":   "RPC",
	"grpc":  "GRPC",
	"uri":   "URI",
	"xml":   "XML",
	"io":    "IO",
	"os":    "OS",
	"tls":   "TLS",
	"ssl":   "SSL",
	"cpu":   "CPU",
	"ram":   "RAM",
	"oauth": "OAuth",
}

var voiceCasingPrefixes = []struct {
	pattern *regexp.Regexp
	style   CasingStyle
}{
	{regexp.MustCompile(`(?i)^(?:snake\s+case|underscore\s+case)\s+(.+)$`), CaseSnake},
	{regexp.MustCompile(`(?i)^(?:camel\s+case)\s+(.+)$`), CaseCamel},
	{regexp.MustCompile(`(?i)^(?:pascal\s+case|title\s+case)\s+(.+)$`), CasePascal},
	{regexp.MustCompile(`(?i)^(?:kebab\s+case|dash\s+case)\s+(.+)$`), CaseKebab},
	{regexp.MustCompile(`(?i)^(?:constant\s+case|screaming\s+snake(?:\s+case)?|all\s+caps\s+case)\s+(.+)$`), CaseConstant},
	{regexp.MustCompile(`(?i)^(?:dot\s+case)\s+(.+)$`), CaseDot},
}

// FormatCode processes text according to the requested code styling, voice casing triggers, or docstrings.
func FormatCode(req CodeFormatRequest) CodeFormatResult {
	raw := strings.TrimSpace(req.Text)
	if raw == "" {
		return CodeFormatResult{Original: "", Result: "", Style: CaseAuto, Matched: false}
	}

	// 1. Docstring generation if requested
	if req.IsDocstring {
		doc := FormatDocstring(raw, req.Language)
		return CodeFormatResult{
			Original: raw,
			Result:   doc,
			Style:    CaseAuto,
			Matched:  true,
		}
	}

	// 2. Expand spoken programming operators first
	expanded := ExpandOperators(raw)

	// 3. Check for voice casing prefixes if style is auto or empty
	style := req.Style
	matched := false
	payload := expanded

	if style == "" || style == CaseAuto {
		for _, v := range voiceCasingPrefixes {
			if matches := v.pattern.FindStringSubmatch(expanded); len(matches) == 2 {
				style = v.style
				payload = strings.TrimSpace(matches[1])
				matched = true
				break
			}
		}
	} else {
		matched = true
	}

	// If no casing style requested or detected, return operator-expanded text
	if style == "" || style == CaseAuto {
		return CodeFormatResult{
			Original: raw,
			Result:   expanded,
			Style:    CaseAuto,
			Matched:  expanded != raw,
		}
	}

	words := splitIntoWords(payload)
	formatted := formatWords(words, style, req.PreserveAcronyms)

	return CodeFormatResult{
		Original: raw,
		Result:   formatted,
		Style:    style,
		Matched:  matched,
	}
}

// ExpandOperators substitutes spoken code operators and programming constructs with exact syntax symbols.
func ExpandOperators(text string) string {
	res := text

	// Replacements in priority order (longer phrases first)
	replacements := []struct {
		pattern *regexp.Regexp
		repl    string
	}{
		// Boolean & comparisons
		{regexp.MustCompile(`(?i)\b(?:error|err)\s+(?:is\s+)?not\s+nil\b`), "err != nil"},
		{regexp.MustCompile(`(?i)\b(?:error|err)\s+(?:is\s+)?nil\b`), "err == nil"},
		{regexp.MustCompile(`(?i)\b(?:greater\s+than\s+or\s+equal(?:\s+to)?|greater\s+than\s+equal)\b`), ">="},
		{regexp.MustCompile(`(?i)\b(?:less\s+than\s+or\s+equal(?:\s+to)?|less\s+than\s+equal)\b`), "<="},
		{regexp.MustCompile(`(?i)\b(?:equals\s+equals|double\s+equals|is\s+equal\s+to)\b`), "=="},
		{regexp.MustCompile(`(?i)\b(?:not\s+equals?|bang\s+equals?|exclamation\s+equals?)\b`), "!="},

		// Logical & bitwise
		{regexp.MustCompile(`(?i)\b(?:double\s+ampersand|and\s+and)\b`), "&&"},
		{regexp.MustCompile(`(?i)\b(?:double\s+pipe|pipe\s+pipe)\b`), "||"},

		// Arrows & assignments
		{regexp.MustCompile(`(?i)\b(?:fat\s+arrow|hash\s+rocket)\b`), "=>"},
		{regexp.MustCompile(`(?i)\b(?:thin\s+arrow|points\s+to)\b`), "->"},
		{regexp.MustCompile(`(?i)\b(?:walrus\s+operator|walrus|colon\s+equals)\b`), ":="},

		// Increments
		{regexp.MustCompile(`(?i)\b(?:plus\s+plus)\b`), "++"},
		{regexp.MustCompile(`(?i)\b(?:minus\s+minus)\b`), "--"},
	}

	for _, r := range replacements {
		res = r.pattern.ReplaceAllString(res, r.repl)
	}

	return res
}

// FormatDocstring transforms descriptive prose into language-specific docstrings.
func FormatDocstring(text, lang string) string {
	clean := strings.TrimSpace(text)
	if clean == "" {
		return ""
	}

	l := strings.ToLower(strings.TrimSpace(lang))
	switch l {
	case "python", "py":
		return fmt.Sprintf(`"""%s"""`, clean)

	case "rust", "rs":
		lines := strings.Split(clean, "\n")
		var out []string
		for _, line := range lines {
			out = append(out, "/// "+line)
		}
		return strings.Join(out, "\n")

	case "javascript", "js", "typescript", "ts", "java", "c", "cpp":
		lines := strings.Split(clean, "\n")
		if len(lines) == 1 {
			return fmt.Sprintf("/** %s */", clean)
		}
		var out []string
		out = append(out, "/**")
		for _, line := range lines {
			out = append(out, " * "+line)
		}
		out = append(out, " */")
		return strings.Join(out, "\n")

	case "go", "golang", "":
		fallthrough
	default:
		lines := strings.Split(clean, "\n")
		var out []string
		for _, line := range lines {
			out = append(out, "// "+line)
		}
		return strings.Join(out, "\n")
	}
}

func formatWords(words []string, style CasingStyle, preserveAcronyms bool) string {
	if len(words) == 0 {
		return ""
	}

	switch style {
	case CaseSnake:
		var lower []string
		for _, w := range words {
			lower = append(lower, strings.ToLower(w))
		}
		return strings.Join(lower, "_")

	case CaseKebab:
		var lower []string
		for _, w := range words {
			lower = append(lower, strings.ToLower(w))
		}
		return strings.Join(lower, "-")

	case CaseConstant:
		var upper []string
		for _, w := range words {
			upper = append(upper, strings.ToUpper(w))
		}
		return strings.Join(upper, "_")

	case CaseDot:
		var lower []string
		for _, w := range words {
			lower = append(lower, strings.ToLower(w))
		}
		return strings.Join(lower, ".")

	case CaseCamel:
		var sb strings.Builder
		for i, w := range words {
			low := strings.ToLower(w)
			if i == 0 {
				sb.WriteString(low)
				continue
			}
			if preserveAcronyms {
				if acr, ok := commonInitialisms[low]; ok {
					sb.WriteString(acr)
					continue
				}
			}
			sb.WriteString(capitalize(low))
		}
		return sb.String()

	case CasePascal:
		var sb strings.Builder
		for _, w := range words {
			low := strings.ToLower(w)
			if preserveAcronyms {
				if acr, ok := commonInitialisms[low]; ok {
					sb.WriteString(acr)
					continue
				}
			}
			sb.WriteString(capitalize(low))
		}
		return sb.String()

	default:
		return strings.Join(words, " ")
	}
}

func splitIntoWords(s string) []string {
	var words []string
	var current strings.Builder

	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
	}

	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
