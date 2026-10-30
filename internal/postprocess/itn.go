package postprocess

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	units = map[string]int64{
		"zero": 0, "one": 1, "two": 2, "three": 3, "four": 4,
		"five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9,
		"ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14,
		"fifteen": 15, "sixteen": 16, "seventeen": 17, "eighteen": 18, "nineteen": 19,
	}

	tens = map[string]int64{
		"twenty": 20, "thirty": 30, "forty": 40, "fifty": 50,
		"sixty": 60, "seventy": 70, "eighty": 80, "ninety": 90,
	}

	scales = map[string]int64{
		"hundred":  100,
		"thousand": 1000,
		"million":  1000000,
		"billion":  1000000000,
	}

	ordinals = map[string]string{
		"first": "1st", "second": "2nd", "third": "3rd", "fourth": "4th", "fifth": "5th",
		"sixth": "6th", "seventh": "7th", "eighth": "8th", "ninth": "9th", "tenth": "10th",
		"eleventh": "11th", "twelfth": "12th", "thirteenth": "13th", "fourteenth": "14th",
		"fifteenth": "15th", "sixteenth": "16th", "seventeenth": "17th", "eighteenth": "18th",
		"nineteenth": "19th", "twentieth": "20th", "thirtieth": "30th", "fortieth": "40th",
		"fiftieth": "50th", "sixtieth": "60th", "seventieth": "70th", "eightieth": "80th",
		"ninetieth": "90th", "hundredth": "100th", "thousandth": "1000th", "millionth": "1000000th",
	}

	currencyRe = regexp.MustCompile(`(?i)\$?(\d+)\s+dollars?\s+and\s+(\d+)\s+cents?\b`)
	dollarRe   = regexp.MustCompile(`(?i)\b(\d+)\s+(dollars?|bucks)\b`)
	centRe     = regexp.MustCompile(`(?i)\b(\d+)\s+cents?\b`)
	euroRe     = regexp.MustCompile(`(?i)\b(\d+)\s+euros?\b`)
	poundRe    = regexp.MustCompile(`(?i)\b(\d+)\s+pounds?\b`)
	percentRe  = regexp.MustCompile(`(?i)\b(\d+)\s+percent\b`)

	timeRe1 = regexp.MustCompile(`(?i)\b([a-z\-]+|\d+)\s+([a-z\-]+|\d+)\s+(am|pm)\b`)
	timeRe2 = regexp.MustCompile(`(?i)\b([a-z\-]+|\d+)\s+(am|pm)\b`)
)

// ITN performs Inverse Text Normalization on spoken transcripts.
type ITN struct{}

// NewITN creates a new Inverse Text Normalization step.
func NewITN() *ITN {
	return &ITN{}
}

// Name returns the step identifier.
func (ITN) Name() string {
	return "itn"
}

// Apply executes ITN transformations on the given text.
func (ITN) Apply(text string, _ Context) string {
	if strings.TrimSpace(text) == "" {
		return text
	}

	text = normalizeTime(text)
	text = normalizeOrdinals(text)
	text = normalizeCardinalNumbers(text)
	text = normalizeCurrency(text)
	text = normalizePercentages(text)

	return text
}

func parseNumberString(s string) (int64, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, true
	}

	parts := strings.Fields(strings.ReplaceAll(s, "-", " "))
	if len(parts) == 0 {
		return 0, false
	}

	var total int64
	var current int64

	for _, w := range parts {
		if val, ok := units[w]; ok {
			current += val
		} else if val, ok := tens[w]; ok {
			current += val
		} else if w == "hundred" {
			if current == 0 {
				current = 1
			}
			current *= 100
		} else if val, ok := scales[w]; ok {
			if current == 0 {
				current = 1
			}
			total += current * val
			current = 0
		} else if w == "and" {
			continue
		} else {
			return 0, false
		}
	}

	return total + current, true
}

func normalizeCurrency(text string) string {
	text = currencyRe.ReplaceAllStringFunc(text, func(m string) string {
		match := currencyRe.FindStringSubmatch(m)
		if len(match) < 3 {
			return m
		}
		dollars, err1 := strconv.ParseInt(match[1], 10, 64)
		cents, err2 := strconv.ParseInt(match[2], 10, 64)
		if err1 == nil && err2 == nil {
			return fmt.Sprintf("$%d.%02d", dollars, cents)
		}
		return m
	})

	text = dollarRe.ReplaceAllStringFunc(text, func(m string) string {
		match := dollarRe.FindStringSubmatch(m)
		if len(match) < 2 {
			return m
		}
		return fmt.Sprintf("$%s", match[1])
	})

	text = centRe.ReplaceAllStringFunc(text, func(m string) string {
		match := centRe.FindStringSubmatch(m)
		if len(match) < 2 {
			return m
		}
		return fmt.Sprintf("%s¢", match[1])
	})

	text = euroRe.ReplaceAllStringFunc(text, func(m string) string {
		match := euroRe.FindStringSubmatch(m)
		if len(match) < 2 {
			return m
		}
		return fmt.Sprintf("€%s", match[1])
	})

	text = poundRe.ReplaceAllStringFunc(text, func(m string) string {
		match := poundRe.FindStringSubmatch(m)
		if len(match) < 2 {
			return m
		}
		return fmt.Sprintf("£%s", match[1])
	})

	return text
}

func normalizePercentages(text string) string {
	return percentRe.ReplaceAllStringFunc(text, func(m string) string {
		match := percentRe.FindStringSubmatch(m)
		if len(match) < 2 {
			return m
		}
		return fmt.Sprintf("%s%%", match[1])
	})
}

func normalizeTime(text string) string {
	text = timeRe1.ReplaceAllStringFunc(text, func(m string) string {
		match := timeRe1.FindStringSubmatch(m)
		if len(match) < 4 {
			return m
		}
		hour, ok1 := parseNumberString(match[1])
		minute, ok2 := parseNumberString(match[2])
		if ok1 && ok2 && hour >= 1 && hour <= 12 && minute >= 0 && minute <= 59 {
			return fmt.Sprintf("%d:%02d %s", hour, minute, strings.ToUpper(match[3]))
		}
		return m
	})

	text = timeRe2.ReplaceAllStringFunc(text, func(m string) string {
		match := timeRe2.FindStringSubmatch(m)
		if len(match) < 3 {
			return m
		}
		hour, ok := parseNumberString(match[1])
		if ok && hour >= 1 && hour <= 12 {
			return fmt.Sprintf("%d %s", hour, strings.ToUpper(match[2]))
		}
		return m
	})

	return text
}

func normalizeOrdinals(text string) string {
	compoundOrdinalRe := regexp.MustCompile(`(?i)\b([a-z]+)[-\s](first|second|third|fourth|fifth|sixth|seventh|eighth|ninth)\b`)
	text = compoundOrdinalRe.ReplaceAllStringFunc(text, func(m string) string {
		parts := compoundOrdinalRe.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		prefix := strings.ToLower(parts[1])
		suffix := strings.ToLower(parts[2])
		if val, ok := tens[prefix]; ok {
			switch suffix {
			case "first":
				return fmt.Sprintf("%dst", val+1)
			case "second":
				return fmt.Sprintf("%dnd", val+2)
			case "third":
				return fmt.Sprintf("%drd", val+3)
			case "fourth":
				return fmt.Sprintf("%dth", val+4)
			case "fifth":
				return fmt.Sprintf("%dth", val+5)
			case "sixth":
				return fmt.Sprintf("%dth", val+6)
			case "seventh":
				return fmt.Sprintf("%dth", val+7)
			case "eighth":
				return fmt.Sprintf("%dth", val+8)
			case "ninth":
				return fmt.Sprintf("%dth", val+9)
			}
		}
		return m
	})

	for word, repl := range ordinals {
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b`)
		text = re.ReplaceAllString(text, repl)
	}

	return text
}

func isNumberWord(w string) bool {
	w = strings.ToLower(w)
	if _, ok := units[w]; ok {
		return true
	}
	if _, ok := tens[w]; ok {
		return true
	}
	if _, ok := scales[w]; ok {
		return true
	}
	if w == "and" {
		return true
	}
	if strings.Contains(w, "-") {
		sub := strings.Split(w, "-")
		if len(sub) == 2 {
			_, ok1 := tens[sub[0]]
			_, ok2 := units[sub[1]]
			return ok1 && ok2
		}
	}
	return false
}

func normalizeCardinalNumbers(text string) string {
	tokens := strings.Fields(text)
	if len(tokens) == 0 {
		return text
	}

	var result []string
	i := 0

	for i < len(tokens) {
		token := tokens[i]
		cleanToken := strings.Trim(strings.ToLower(token), ",.?!:;")

		if isNumberWord(cleanToken) && cleanToken != "and" {
			j := i
			var numberWords []string

			for j < len(tokens) {
				w := strings.Trim(strings.ToLower(tokens[j]), ",.?!:;")
				if isNumberWord(w) {
					numberWords = append(numberWords, w)
					if strings.ContainsAny(tokens[j], ",.?!:;") {
						j++
						break
					}
					j++
				} else {
					break
				}
			}

			for len(numberWords) > 0 && numberWords[len(numberWords)-1] == "and" {
				numberWords = numberWords[:len(numberWords)-1]
				j--
			}

			if num, ok := parseNumberString(strings.Join(numberWords, " ")); ok {
				lastToken := tokens[j-1]
				punct := ""
				for k := len(lastToken) - 1; k >= 0; k-- {
					if strings.ContainsRune(",.?!:;", rune(lastToken[k])) {
						punct = string(lastToken[k]) + punct
					} else {
						break
					}
				}
				result = append(result, fmt.Sprintf("%d%s", num, punct))
				i = j
				continue
			}
		}

		result = append(result, token)
		i++
	}

	return strings.Join(result, " ")
}
