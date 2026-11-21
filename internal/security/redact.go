package security

import (
	"regexp"
	"strings"
)

var (
	// Redaction patterns
	apiKeyRegex     = regexp.MustCompile(`\b(sk-[a-zA-Z0-9_\-]{20,}|ghp_[a-zA-Z0-9]{20,}|Bearer\s+[a-zA-Z0-9_\-\.]{20,})\b`)
	creditCardRegex = regexp.MustCompile(`\b(?:\d{4}[ -]?){3}\d{4}\b`)
	ssnRegex        = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	passwordRegex   = regexp.MustCompile(`(?i)\b(password|passwd|secret|api[_-]?key)\s*[:=]\s*([^\s,;]+)`)
)

// RedactText sanitizes a text string by masking credit cards, SSNs, API keys, and passwords.
func RedactText(input string) string {
	if input == "" {
		return ""
	}

	result := input

	// Redact API keys and tokens
	result = apiKeyRegex.ReplaceAllString(result, "[REDACTED_KEY]")

	// Redact credit cards
	result = creditCardRegex.ReplaceAllString(result, "[REDACTED_CC]")

	// Redact SSNs
	result = ssnRegex.ReplaceAllString(result, "[REDACTED_SSN]")

	// Redact password assignments
	result = passwordRegex.ReplaceAllStringFunc(result, func(match string) string {
		parts := strings.SplitN(match, ":", 2)
		if len(parts) == 2 {
			return parts[0] + ": [REDACTED]"
		}
		parts = strings.SplitN(match, "=", 2)
		if len(parts) == 2 {
			return parts[0] + "= [REDACTED]"
		}
		return "[REDACTED_SECRET]"
	})

	return result
}
