package edit

import (
	"context"
	"testing"
)

func TestParser_Parse(t *testing.T) {
	parser := NewParser(nil)
	ctx := context.Background()

	tests := []struct {
		name           string
		input          string
		wantIntent     Intent
		wantIsEdit     bool
		wantLang       string
		wantTone       string
		wantFormat     string
		wantConfidence float64
	}{
		{
			name:       "Empty string",
			input:      "",
			wantIntent: IntentNone,
			wantIsEdit: false,
		},
		{
			name:       "Regular dictation sentence",
			input:      "the quarterly earnings exceeded our expectations by 15 percent",
			wantIntent: IntentNone,
			wantIsEdit: false,
		},
		{
			name:       "Regular question",
			input:      "what time is the project status meeting today?",
			wantIntent: IntentNone,
			wantIsEdit: false,
		},
		{
			name:       "Translate to French",
			input:      "translate this to French",
			wantIntent: IntentTranslate,
			wantIsEdit: true,
			wantLang:   "French",
		},
		{
			name:       "Please translate into Spanish",
			input:      "please translate into Spanish",
			wantIntent: IntentTranslate,
			wantIsEdit: true,
			wantLang:   "Spanish",
		},
		{
			name:       "Say this in German please",
			input:      "say this in German please",
			wantIntent: IntentTranslate,
			wantIsEdit: true,
			wantLang:   "German",
		},
		{
			name:       "Summarize this",
			input:      "summarize this",
			wantIntent: IntentSummarize,
			wantIsEdit: true,
		},
		{
			name:       "Summarize in bullet points",
			input:      "give me a summary of this in bullet points",
			wantIntent: IntentSummarize,
			wantIsEdit: true,
			wantFormat: "bullet_list",
		},
		{
			name:       "TLDR this",
			input:      "tldr this",
			wantIntent: IntentSummarize,
			wantIsEdit: true,
		},
		{
			name:       "Key takeaways",
			input:      "key takeaways from this",
			wantIntent: IntentSummarize,
			wantIsEdit: true,
		},
		{
			name:       "Make this shorter",
			input:      "make this shorter",
			wantIntent: IntentShorten,
			wantIsEdit: true,
			wantTone:   "concise",
		},
		{
			name:       "Condense this",
			input:      "condense this",
			wantIntent: IntentShorten,
			wantIsEdit: true,
			wantTone:   "concise",
		},
		{
			name:       "Cut this down",
			input:      "cut this down",
			wantIntent: IntentShorten,
			wantIsEdit: true,
			wantTone:   "concise",
		},
		{
			name:       "Expand on this",
			input:      "expand on this",
			wantIntent: IntentExpand,
			wantIsEdit: true,
		},
		{
			name:       "Make this longer",
			input:      "make this longer",
			wantIntent: IntentExpand,
			wantIsEdit: true,
		},
		{
			name:       "Add more details to this",
			input:      "add more details to this",
			wantIntent: IntentExpand,
			wantIsEdit: true,
		},
		{
			name:       "Format as table",
			input:      "turn this into a markdown table",
			wantIntent: IntentFormatTable,
			wantIsEdit: true,
			wantFormat: "table",
		},
		{
			name:       "Format as table with columns",
			input:      "turn this into a table with columns Name, Role, Team",
			wantIntent: IntentFormatTable,
			wantIsEdit: true,
			wantFormat: "table",
		},
		{
			name:       "Tabularize this",
			input:      "tabularize this",
			wantIntent: IntentFormatTable,
			wantIsEdit: true,
			wantFormat: "table",
		},
		{
			name:       "Format as checklist",
			input:      "make this into a checklist",
			wantIntent: IntentFormatList,
			wantIsEdit: true,
			wantFormat: "checklist",
		},
		{
			name:       "Format as bullet list",
			input:      "convert this into a bullet list",
			wantIntent: IntentFormatList,
			wantIsEdit: true,
			wantFormat: "bullet_list",
		},
		{
			name:       "Format as numbered list",
			input:      "format this as a numbered list",
			wantIntent: IntentFormatList,
			wantIsEdit: true,
			wantFormat: "numbered_list",
		},
		{
			name:       "Rewrite to sound professional",
			input:      "rewrite this to sound professional",
			wantIntent: IntentRewrite,
			wantIsEdit: true,
			wantTone:   "professional",
		},
		{
			name:       "Change tone to casual",
			input:      "change tone to casual",
			wantIntent: IntentRewrite,
			wantIsEdit: true,
			wantTone:   "casual",
		},
		{
			name:       "Fix grammar and spelling",
			input:      "fix grammar and spelling in this",
			wantIntent: IntentRewrite,
			wantIsEdit: true,
		},
		{
			name:       "Custom edit instruction",
			input:      "edit the third paragraph to mention budget approval",
			wantIntent: IntentCustom,
			wantIsEdit: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := parser.Parse(ctx, tc.input)

			if res.Intent != tc.wantIntent {
				t.Errorf("Intent = %q, want %q", res.Intent, tc.wantIntent)
			}
			if res.IsEdit != tc.wantIsEdit {
				t.Errorf("IsEdit = %v, want %v", res.IsEdit, tc.wantIsEdit)
			}
			if tc.wantLang != "" && res.TargetLanguage != tc.wantLang {
				t.Errorf("TargetLanguage = %q, want %q", res.TargetLanguage, tc.wantLang)
			}
			if tc.wantTone != "" && res.Tone != tc.wantTone {
				t.Errorf("Tone = %q, want %q", res.Tone, tc.wantTone)
			}
			if tc.wantFormat != "" && res.Format != tc.wantFormat {
				t.Errorf("Format = %q, want %q", res.Format, tc.wantFormat)
			}
			if res.Raw != tc.input {
				t.Errorf("Raw = %q, want %q", res.Raw, tc.input)
			}
		})
	}

	t.Run("Table columns extraction", func(t *testing.T) {
		res := parser.Parse(ctx, "turn this into a table with columns Name, Role, Team")
		if len(res.Columns) != 3 || res.Columns[0] != "Name" || res.Columns[1] != "Role" || res.Columns[2] != "Team" {
			t.Errorf("unexpected columns: %v", res.Columns)
		}
	})
}
