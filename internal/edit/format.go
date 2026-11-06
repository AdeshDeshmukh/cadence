package edit

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	reItemSplit   = regexp.MustCompile(`(?:\r?\n|;|\s*,\s*(?:and\s+)?|\s+and\s+)`)
	rePrefixTrim  = regexp.MustCompile(`^(?:[-*•+]|\d+[\.)]|\[[ xX]?\])\s*`)
	reTableDelim  = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)+\|?\s*$`)
	reTableColSep = regexp.MustCompile(`\s*\|\s*`)
)

// FormatAsList formats unstructured or delimited text into a Markdown list.
// Formats: "bullet_list", "numbered_list", "checklist".
func FormatAsList(text string, format string) string {
	rawLines := strings.Split(text, "\n")
	var items []string

	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// If line already looks like a list item, extract the content
		cleanItem := rePrefixTrim.ReplaceAllString(trimmed, "")
		cleanItem = strings.TrimSpace(cleanItem)
		if cleanItem != "" {
			items = append(items, cleanItem)
		}
	}

	// If it was just a single line with commas or "and", split into items
	if len(items) <= 1 && len(text) > 0 {
		candidates := reItemSplit.Split(text, -1)
		var refined []string
		for _, c := range candidates {
			cl := strings.TrimSpace(rePrefixTrim.ReplaceAllString(strings.TrimSpace(c), ""))
			if cl != "" {
				refined = append(refined, cl)
			}
		}
		if len(refined) > 1 {
			items = refined
		}
	}

	if len(items) == 0 {
		return text
	}

	var sb strings.Builder
	for i, item := range items {
		switch format {
		case "numbered_list":
			fmt.Fprintf(&sb, "%d. %s\n", i+1, item)
		case "checklist":
			fmt.Fprintf(&sb, "- [ ] %s\n", item)
		default: // "bullet_list"
			fmt.Fprintf(&sb, "- %s\n", item)
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// FormatAsTable formats rows of strings into an aligned Markdown table.
func FormatAsTable(rows [][]string, headers []string) string {
	if len(rows) == 0 && len(headers) == 0 {
		return ""
	}

	numCols := len(headers)
	for _, r := range rows {
		if len(r) > numCols {
			numCols = len(r)
		}
	}
	if numCols == 0 {
		return ""
	}

	// Pad headers if shorter than numCols
	normHeaders := make([]string, numCols)
	for i := 0; i < numCols; i++ {
		if i < len(headers) && strings.TrimSpace(headers[i]) != "" {
			normHeaders[i] = strings.TrimSpace(headers[i])
		} else {
			normHeaders[i] = fmt.Sprintf("Column %d", i+1)
		}
	}

	// Pad rows
	var normRows [][]string
	for _, r := range rows {
		row := make([]string, numCols)
		for i := 0; i < numCols; i++ {
			if i < len(r) {
				row[i] = strings.TrimSpace(r[i])
			} else {
				row[i] = ""
			}
		}
		normRows = append(normRows, row)
	}

	// Calculate column widths
	widths := make([]int, numCols)
	for i := 0; i < numCols; i++ {
		widths[i] = len(normHeaders[i])
		if widths[i] < 3 {
			widths[i] = 3 // Minimum width for '---'
		}
		for _, r := range normRows {
			if len(r[i]) > widths[i] {
				widths[i] = len(r[i])
			}
		}
	}

	var sb strings.Builder

	// Header row
	sb.WriteString("| ")
	for i, h := range normHeaders {
		sb.WriteString(padRight(h, widths[i]))
		if i < numCols-1 {
			sb.WriteString(" | ")
		}
	}
	sb.WriteString(" |\n")

	// Separator row
	sb.WriteString("| ")
	for i := 0; i < numCols; i++ {
		sb.WriteString(strings.Repeat("-", widths[i]))
		if i < numCols-1 {
			sb.WriteString(" | ")
		}
	}
	sb.WriteString(" |\n")

	// Data rows
	for _, r := range normRows {
		sb.WriteString("| ")
		for i, cell := range r {
			sb.WriteString(padRight(cell, widths[i]))
			if i < numCols-1 {
				sb.WriteString(" | ")
			}
		}
		sb.WriteString(" |\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

// ValidateMarkdownTable checks if a string is a well-formed Markdown table.
func ValidateMarkdownTable(s string) bool {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) < 2 {
		return false
	}

	header := strings.TrimSpace(lines[0])
	delim := strings.TrimSpace(lines[1])

	if !strings.Contains(header, "|") {
		return false
	}
	return reTableDelim.MatchString(delim)
}

// AlignMarkdownTable parses any markdown table and reformats it with aligned column widths.
func AlignMarkdownTable(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) < 2 || !ValidateMarkdownTable(s) {
		return s
	}

	var rows [][]string
	for i, line := range lines {
		if i == 1 {
			// Skip delimiter line
			continue
		}
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimPrefix(trimmed, "|")
		trimmed = strings.TrimSuffix(trimmed, "|")
		parts := strings.Split(trimmed, "|")
		var cleanParts []string
		for _, p := range parts {
			cleanParts = append(cleanParts, strings.TrimSpace(p))
		}
		rows = append(rows, cleanParts)
	}

	if len(rows) == 0 {
		return s
	}

	headers := rows[0]
	dataRows := rows[1:]
	return FormatAsTable(dataRows, headers)
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
