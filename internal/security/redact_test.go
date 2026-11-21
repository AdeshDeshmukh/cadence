package security

import (
	"strings"
	"testing"
)

func TestRedactText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty",
			input:    "",
			expected: "",
		},
		{
			name:     "clean text without secrets",
			input:    "Hello world this is a normal dictation transcript.",
			expected: "Hello world this is a normal dictation transcript.",
		},
		{
			name:     "OpenAI API key",
			input:    "My key is sk-1234567890abcdef1234567890 and use it for dictation",
			expected: "My key is [REDACTED_KEY] and use it for dictation",
		},
		{
			name:     "GitHub token",
			input:    "Push with token ghp_1234567890abcdef1234567890 please",
			expected: "Push with token [REDACTED_KEY] please",
		},
		{
			name:     "Credit card with hyphens",
			input:    "Card number 4111-2222-3333-4444 expires next month",
			expected: "Card number [REDACTED_CC] expires next month",
		},
		{
			name:     "Credit card with spaces",
			input:    "Charged to 5500 0000 0000 0004 yesterday",
			expected: "Charged to [REDACTED_CC] yesterday",
		},
		{
			name:     "Social security number",
			input:    "Client SSN is 123-45-6789 in the record",
			expected: "Client SSN is [REDACTED_SSN] in the record",
		},
		{
			name:     "Password assignment colon",
			input:    "Connect with password: SuperSecretP@ssw0rd! now",
			expected: "Connect with password: [REDACTED] now",
		},
		{
			name:     "Password assignment equals",
			input:    "Database url postgres://user:admin with secret=my_db_secret_key_123",
			expected: "Database url postgres://user:admin with secret= [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactText(tt.input)
			if !strings.EqualFold(got, tt.expected) {
				t.Errorf("RedactText() = %q, expected %q", got, tt.expected)
			}
		})
	}
}
