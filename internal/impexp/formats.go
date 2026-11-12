package impexp

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Format represents a supported serialization/deserialization format.
type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatSRT      Format = "srt"
	FormatVTT      Format = "vtt"
	FormatJSON     Format = "json"
	FormatTXT      Format = "txt"
)

// NormalizeFormat parses a format string or extension into a standardized Format.
func NormalizeFormat(f string) (Format, error) {
	clean := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(f, ".")))
	switch clean {
	case "markdown", "md":
		return FormatMarkdown, nil
	case "srt", "subrip":
		return FormatSRT, nil
	case "vtt", "webvtt":
		return FormatVTT, nil
	case "json":
		return FormatJSON, nil
	case "txt", "text", "plain":
		return FormatTXT, nil
	default:
		return "", fmt.Errorf("unsupported format %q (supported: markdown, srt, vtt, json, txt)", f)
	}
}

// FormatSRTTime formats milliseconds into standard SRT timestamp format: HH:MM:SS,mmm
func FormatSRTTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	d := time.Duration(ms) * time.Millisecond
	hours := int64(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	minutes := int64(d / time.Minute)
	d -= time.Duration(minutes) * time.Minute
	seconds := int64(d / time.Second)
	d -= time.Duration(seconds) * time.Second
	millis := int64(d / time.Millisecond)

	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}

// FormatVTTTime formats milliseconds into standard WebVTT timestamp format: HH:MM:SS.mmm
func FormatVTTTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	d := time.Duration(ms) * time.Millisecond
	hours := int64(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	minutes := int64(d / time.Minute)
	d -= time.Duration(minutes) * time.Minute
	seconds := int64(d / time.Second)
	d -= time.Duration(seconds) * time.Second
	millis := int64(d / time.Millisecond)

	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}

// FormatDuration formats milliseconds into human-readable duration string (e.g. "1h 23m 45s", "12m 30s", "45s")
func FormatDuration(ms int64) string {
	if ms <= 0 {
		return "0s"
	}
	d := time.Duration(ms) * time.Millisecond
	hours := int64(d / time.Hour)
	d -= time.Duration(hours) * time.Hour
	minutes := int64(d / time.Minute)
	d -= time.Duration(minutes) * time.Minute
	seconds := int64(d / time.Second)

	parts := []string{}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 || hours > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	parts = append(parts, fmt.Sprintf("%ds", seconds))
	return strings.Join(parts, " ")
}

var srtTimeRegex = regexp.MustCompile(`^(\d+):(\d{2}):(\d{2})[,.](\d{3})$`)
var vttShortTimeRegex = regexp.MustCompile(`^(\d{2}):(\d{2})[,.](\d{3})$`)

// ParseTimestamp parses a timestamp string (HH:MM:SS,mmm or HH:MM:SS.mmm or MM:SS.mmm) into milliseconds.
func ParseTimestamp(ts string) (int64, error) {
	ts = strings.TrimSpace(ts)
	if m := srtTimeRegex.FindStringSubmatch(ts); m != nil {
		h, _ := strconv.ParseInt(m[1], 10, 64)
		min, _ := strconv.ParseInt(m[2], 10, 64)
		sec, _ := strconv.ParseInt(m[3], 10, 64)
		ms, _ := strconv.ParseInt(m[4], 10, 64)
		return (h*3600+min*60+sec)*1000 + ms, nil
	}
	if m := vttShortTimeRegex.FindStringSubmatch(ts); m != nil {
		min, _ := strconv.ParseInt(m[1], 10, 64)
		sec, _ := strconv.ParseInt(m[2], 10, 64)
		ms, _ := strconv.ParseInt(m[3], 10, 64)
		return (min*60+sec)*1000 + ms, nil
	}
	return 0, fmt.Errorf("invalid timestamp format: %q", ts)
}
