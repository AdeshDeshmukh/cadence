package developer

import (
	"strings"
	"testing"
)

func TestCasing_Styles(t *testing.T) {
	words := []string{"user", "profile", "account"}

	// 1. Snake case
	snake := formatWords(words, CaseSnake, false)
	if snake != "user_profile_account" {
		t.Fatalf("expected user_profile_account, got %s", snake)
	}

	// 2. Kebab case
	kebab := formatWords(words, CaseKebab, false)
	if kebab != "user-profile-account" {
		t.Fatalf("expected user-profile-account, got %s", kebab)
	}

	// 3. Constant case
	constant := formatWords(words, CaseConstant, false)
	if constant != "USER_PROFILE_ACCOUNT" {
		t.Fatalf("expected USER_PROFILE_ACCOUNT, got %s", constant)
	}

	// 4. Dot case
	dot := formatWords(words, CaseDot, false)
	if dot != "user.profile.account" {
		t.Fatalf("expected user.profile.account, got %s", dot)
	}

	// 5. Camel case
	camel := formatWords(words, CaseCamel, false)
	if camel != "userProfileAccount" {
		t.Fatalf("expected userProfileAccount, got %s", camel)
	}

	// 6. Pascal case
	pascal := formatWords(words, CasePascal, false)
	if pascal != "UserProfileAccount" {
		t.Fatalf("expected UserProfileAccount, got %s", pascal)
	}
}

func TestCasing_AcronymPreservation(t *testing.T) {
	words := []string{"get", "http", "api", "url"}

	// Without acronym preservation
	camelNoAcr := formatWords(words, CaseCamel, false)
	if camelNoAcr != "getHttpApiUrl" {
		t.Fatalf("expected getHttpApiUrl, got %s", camelNoAcr)
	}

	// With acronym preservation
	camelAcr := formatWords(words, CaseCamel, true)
	if camelAcr != "getHTTPAPIURL" {
		t.Fatalf("expected getHTTPAPIURL, got %s", camelAcr)
	}

	pascalAcr := formatWords(words, CasePascal, true)
	if pascalAcr != "GetHTTPAPIURL" {
		t.Fatalf("expected GetHTTPAPIURL, got %s", pascalAcr)
	}
}

func TestFormatCode_VoicePrefixes(t *testing.T) {
	tests := []struct {
		input       string
		expected    string
		expectStyle CasingStyle
	}{
		{"snake case user account id", "user_account_id", CaseSnake},
		{"camel case get account balance", "getAccountBalance", CaseCamel},
		{"pascal case auth token provider", "AuthTokenProvider", CasePascal},
		{"kebab case api gateway route", "api-gateway-route", CaseKebab},
		{"constant case max retry limit", "MAX_RETRY_LIMIT", CaseConstant},
		{"screaming snake max pool size", "MAX_POOL_SIZE", CaseConstant},
		{"dot case cadence config db", "cadence.config.db", CaseDot},
	}

	for _, tt := range tests {
		res := FormatCode(CodeFormatRequest{
			Text:             tt.input,
			Style:            CaseAuto,
			PreserveAcronyms: false,
		})
		if !res.Matched {
			t.Errorf("expected %q to match voice prefix", tt.input)
		}
		if res.Result != tt.expected {
			t.Errorf("input %q: expected %q, got %q", tt.input, tt.expected, res.Result)
		}
		if res.Style != tt.expectStyle {
			t.Errorf("input %q: expected style %s, got %s", tt.input, tt.expectStyle, res.Style)
		}
	}
}

func TestExpandOperators(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"if err not nil return err", "if err != nil return err"},
		{"if error is nil continue", "if err == nil continue"},
		{"a equals equals b and and c not equals d", "a == b && c != d"},
		{"flag pipe pipe otherFlag", "flag || otherFlag"},
		{"x fat arrow y", "x => y"},
		{"val points to next", "val -> next"},
		{"val walrus operator 100", "val := 100"},
		{"counter plus plus", "counter ++"},
		{"index minus minus", "index --"},
		{"val greater than or equal to 10", "val >= 10"},
		{"val less than or equal to 5", "val <= 5"},
	}

	for _, tt := range tests {
		out := ExpandOperators(tt.input)
		if out != tt.expected {
			t.Errorf("ExpandOperators(%q): expected %q, got %q", tt.input, tt.expected, out)
		}
	}
}

func TestFormatDocstring(t *testing.T) {
	// 1. Go docstring
	goDoc := FormatDocstring("Execute performs the operation on context.", "go")
	if goDoc != "// Execute performs the operation on context." {
		t.Fatalf("unexpected go doc: %s", goDoc)
	}

	// 2. Python docstring
	pyDoc := FormatDocstring("Calculates the total balance.", "python")
	if pyDoc != `"""Calculates the total balance."""` {
		t.Fatalf("unexpected py doc: %s", pyDoc)
	}

	// 3. TypeScript / JS multiline docstring
	tsDoc := FormatDocstring("Line one.\nLine two.", "ts")
	if !strings.Contains(tsDoc, "/**") || !strings.Contains(tsDoc, " * Line one.") {
		t.Fatalf("unexpected ts doc: %s", tsDoc)
	}

	// 4. Rust docstring
	rsDoc := FormatDocstring("Returns a new instance.", "rust")
	if rsDoc != "/// Returns a new instance." {
		t.Fatalf("unexpected rs doc: %s", rsDoc)
	}
}
