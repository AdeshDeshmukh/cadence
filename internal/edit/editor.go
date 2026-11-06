package edit

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/AdeshDeshmukh/cadence/internal/polish"
)

// EditResult contains the outcome of an in-place voice edit transformation.
type EditResult struct {
	OriginalText string        `json:"original_text"`
	EditedText   string        `json:"edited_text"`
	Instruction  Instruction   `json:"instruction"`
	Model        string        `json:"model,omitempty"`
	Latency      time.Duration `json:"latency"`
}

// Editor transforms selected text based on structured voice instructions.
type Editor struct {
	polisher polish.Polisher
}

// NewEditor creates a new voice text editor.
func NewEditor(polisher polish.Polisher) *Editor {
	if polisher == nil {
		polisher = polish.NewNoopPolisher()
	}
	return &Editor{
		polisher: polisher,
	}
}

// Execute performs the voice edit transformation on the given input text.
func (e *Editor) Execute(ctx context.Context, text string, instr Instruction) (EditResult, error) {
	cleanText := strings.TrimSpace(text)
	if cleanText == "" {
		return EditResult{}, errors.New("cannot edit empty text")
	}

	start := time.Now()

	systemPrompt, userPrompt := buildEditPrompts(cleanText, instr)

	req := polish.Request{
		Text: userPrompt,
		Metadata: map[string]string{
			"system_prompt": systemPrompt,
			"intent":        string(instr.Intent),
		},
	}

	resp, err := e.polisher.Polish(ctx, req)
	if err != nil {
		return EditResult{}, fmt.Errorf("polisher error: %w", err)
	}

	rawOutput := resp.Text
	if e.polisher.Name() == "noop" || resp.Model == "noop" {
		// Pass-through fallback when LLM is inactive
		rawOutput = cleanText
	}

	edited := cleanLLMOutput(rawOutput)

	// Preserve surrounding whitespace from the original selection if present
	edited = preserveSurroundingWhitespace(text, edited)

	return EditResult{
		OriginalText: text,
		EditedText:   edited,
		Instruction:  instr,
		Model:        resp.Model,
		Latency:      time.Since(start),
	}, nil
}

// buildEditPrompts constructs specialized instructions and prompt for the LLM.
func buildEditPrompts(text string, instr Instruction) (string, string) {
	systemPrompt := `You are an expert precision text editing engine.
Your sole job is to transform the provided user text according to the specific instruction.
CRITICAL RULES:
1. Output ONLY the resulting transformed text.
2. NEVER include conversational prefaces (e.g. "Here is...", "Sure!").
3. NEVER include explanations, notes, or meta-commentary.
4. Do NOT wrap output in markdown quotes or backtick code blocks unless specifically requested (like markdown tables).`

	var instruction string

	switch instr.Intent {
	case IntentRewrite:
		if instr.Tone != "" {
			instruction = fmt.Sprintf("Rewrite the text to sound %s. Preserve facts, core ideas, and specific names/terms, but adapt the style, phrasing, and vocabulary.", instr.Tone)
		} else if instr.Prompt != "" {
			instruction = fmt.Sprintf("Rewrite the text following these instructions: %s", instr.Prompt)
		} else {
			instruction = "Rewrite the text to improve clarity, flow, grammar, and sentence structure while preserving exact meaning."
		}

	case IntentShorten:
		instruction = "Condense and shorten the text. Remove wordiness, filler, and redundancy while preserving all key information, essential facts, and numbers."

	case IntentExpand:
		instruction = "Elaborate and expand upon the text. Add depth, detail, and clarity to the ideas while maintaining the original tone and subject matter."

	case IntentSummarize:
		if instr.Format == "bullet_list" {
			instruction = "Summarize the key points of the text into a clean, concise bulleted list."
		} else {
			instruction = "Provide a concise summary capturing the core message and most important takeaways of the text."
		}

	case IntentTranslate:
		targetLang := instr.TargetLanguage
		if targetLang == "" {
			targetLang = "English"
		}
		instruction = fmt.Sprintf("Translate the text into %s. Maintain the exact nuance, tone, and formatting of the original.", targetLang)

	case IntentFormatTable:
		instruction = "Convert the information in the text into a clean Markdown table with headers and row separators."

	case IntentFormatList:
		if instr.Format == "numbered_list" {
			instruction = "Format the text into a clean numbered list."
		} else {
			instruction = "Format the text into a clean bulleted list."
		}

	case IntentCustom:
		instruction = fmt.Sprintf("Edit the text according to this instruction: %s", instr.Prompt)

	default:
		instruction = "Polish and format the text for maximum clarity and correctness."
	}

	userPrompt := fmt.Sprintf("Instruction: %s\n\nText to Edit:\n%s", instruction, text)
	return systemPrompt, userPrompt
}

var (
	rePreface = regexp.MustCompile(`(?i)^(?:here\s+is\s+(?:the\s+)?(?:rewritten|edited|shortened|expanded|translated|summarized)?\s*(?:text|version|result)?:\s*)+`)
)

// cleanLLMOutput cleans up common unwanted LLM artifacts like prefaces and wrapper quotes.
func cleanLLMOutput(out string) string {
	res := strings.TrimSpace(out)

	// Remove conversational intro preface if present
	res = rePreface.ReplaceAllString(res, "")
	res = strings.TrimSpace(res)

	// Strip surrounding double quotes if the whole text was quoted
	if len(res) >= 2 && ((res[0] == '"' && res[len(res)-1] == '"') || (strings.HasPrefix(res, "“") && strings.HasSuffix(res, "”"))) {
		res = strings.TrimPrefix(res, "\"")
		res = strings.TrimSuffix(res, "\"")
		res = strings.TrimPrefix(res, "“")
		res = strings.TrimSuffix(res, "”")
		res = strings.TrimSpace(res)
	}

	return res
}

// preserveSurroundingWhitespace ensures leading/trailing whitespace of the original selection is kept.
func preserveSurroundingWhitespace(original, edited string) string {
	var leading, trailing string

	for i := 0; i < len(original); i++ {
		if original[i] == ' ' || original[i] == '\t' || original[i] == '\n' || original[i] == '\r' {
			leading += string(original[i])
		} else {
			break
		}
	}

	for i := len(original) - 1; i >= 0; i-- {
		if original[i] == ' ' || original[i] == '\t' || original[i] == '\n' || original[i] == '\r' {
			trailing = string(original[i]) + trailing
		} else {
			break
		}
	}

	return leading + strings.TrimSpace(edited) + trailing
}
