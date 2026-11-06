package edit

import (
	"strings"
	"testing"
)

func TestFormatAsList(t *testing.T) {
	t.Run("Bullet list from newlines", func(t *testing.T) {
		input := "buy milk\npick up dry cleaning\nschedule dental appointment"
		res := FormatAsList(input, "bullet_list")
		expected := "- buy milk\n- pick up dry cleaning\n- schedule dental appointment"
		if res != expected {
			t.Errorf("got:\n%s\nwant:\n%s", res, expected)
		}
	})

	t.Run("Numbered list from commas", func(t *testing.T) {
		input := "apples, oranges, and bananas"
		res := FormatAsList(input, "numbered_list")
		expected := "1. apples\n2. oranges\n3. bananas"
		if res != expected {
			t.Errorf("got:\n%s\nwant:\n%s", res, expected)
		}
	})

	t.Run("Checklist from existing bullet markers", func(t *testing.T) {
		input := "* task one\n* task two\n- task three"
		res := FormatAsList(input, "checklist")
		expected := "- [ ] task one\n- [ ] task two\n- [ ] task three"
		if res != expected {
			t.Errorf("got:\n%s\nwant:\n%s", res, expected)
		}
	})
}

func TestFormatAsTable(t *testing.T) {
	headers := []string{"Name", "Role", "Department"}
	rows := [][]string{
		{"Alice Smith", "Engineer", "Infrastructure"},
		{"Bob Jones", "Designer", "UX"},
	}

	res := FormatAsTable(rows, headers)
	if !strings.Contains(res, "| Name        | Role     | Department     |") {
		t.Errorf("unexpected header format:\n%s", res)
	}
	if !strings.Contains(res, "| ----------- | -------- | -------------- |") {
		t.Errorf("unexpected separator format:\n%s", res)
	}
	if !strings.Contains(res, "| Alice Smith | Engineer | Infrastructure |") {
		t.Errorf("unexpected row format:\n%s", res)
	}
}

func TestValidateAndAlignMarkdownTable(t *testing.T) {
	validTable := `| Item | Price |
|---|---|
| Widget | $10 |
| Gadget | $25 |`

	if !ValidateMarkdownTable(validTable) {
		t.Errorf("expected table to be valid")
	}

	aligned := AlignMarkdownTable(validTable)
	if !strings.Contains(aligned, "| Item   | Price |") {
		t.Errorf("unexpected aligned output:\n%s", aligned)
	}

	invalidTable := "This is just regular text with a | vertical bar"
	if ValidateMarkdownTable(invalidTable) {
		t.Errorf("expected invalid table to fail validation")
	}
}
