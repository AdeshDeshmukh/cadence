package postprocess

import (
	"regexp"
	"strings"
	"sync"

	appcontext "github.com/AdeshDeshmukh/cadence/internal/context"
)

// CorrectionRule associates a regex pattern with a canonical replacement string.
type CorrectionRule struct {
	Pattern     *regexp.Regexp
	Replacement string
}

// CorrectionReport details the outcome of an ASR self-correction check.
type CorrectionReport struct {
	Original           string              `json:"original"`
	Corrected          string              `json:"corrected"`
	App                string              `json:"app,omitempty"`
	Category           appcontext.Category `json:"category"`
	CorrectionsApplied int                 `json:"corrections_applied"`
}

var (
	generalRules []CorrectionRule
	codeRules    []CorrectionRule
	medicalRules []CorrectionRule
	legalRules   []CorrectionRule
	rulesOnce    sync.Once
)

func initBuiltinRules() {
	rulesOnce.Do(func() {
		// General rules
		genPairs := [][2]string{
			{`\bcould of\b`, "could have"},
			{`\bwould of\b`, "would have"},
			{`\bshould of\b`, "should have"},
			{`\bnever the less\b`, "nevertheless"},
			{`\bnone the less\b`, "nonetheless"},
			{`\bevery time\b`, "every time"},
			{`\ball lot\b`, "a lot"},
		}
		for _, pair := range genPairs {
			generalRules = append(generalRules, CorrectionRule{
				Pattern:     regexp.MustCompile(`(?i)` + pair[0]),
				Replacement: pair[1],
			})
		}

		// Code & Terminal rules
		codePairs := [][2]string{
			{`\bfour loop\b`, "for loop"},
			{`\bfore loop\b`, "for loop"},
			{`\bfour each\b`, "foreach"},
			{`\bfore each\b`, "foreach"},
			{`\bsink await\b`, "async await"},
			{`\bsync await\b`, "async await"},
			{`\bpie test\b`, "pytest"},
			{`\bgit check out\b`, "git checkout"},
			{`\bgit check in\b`, "git commit"},
			{`\bgit commit minus m\b`, `git commit -m`},
			{`\bpseudo code\b`, "pseudocode"},
			{`\bdot net\b`, ".NET"},
			{`\bcall back\b`, "callback"},
			{`\bcall backs\b`, "callbacks"},
			{`\bweb pack\b`, "webpack"},
			{`\bcube cuddle\b`, "kubectl"},
			{`\bcube control\b`, "kubectl"},
			{`\bdocker compose\b`, "docker-compose"},
			{`\bstandard in\b`, "stdin"},
			{`\bstandard out\b`, "stdout"},
			{`\bstandard error\b`, "stderr"},
			{`\bstandard err\b`, "stderr"},
			{`\bcamel case\b`, "camelCase"},
			{`\bsnake case\b`, "snake_case"},
			{`\bkebab case\b`, "kebab-case"},
			{`\bgo routine\b`, "goroutine"},
			{`\bgo routines\b`, "goroutines"},
			{`\bread me\b`, "README"},
			{`\bfront end\b`, "frontend"},
			{`\bback end\b`, "backend"},
			{`\bfull stack\b`, "fullstack"},
			{`\bfile system\b`, "filesystem"},
			{`\bdata base\b`, "database"},
			{`\bdata bases\b`, "databases"},
			{`\bapi key\b`, "API key"},
			{`\bapi keys\b`, "API keys"},
		}
		for _, pair := range codePairs {
			codeRules = append(codeRules, CorrectionRule{
				Pattern:     regexp.MustCompile(`(?i)` + pair[0]),
				Replacement: pair[1],
			})
		}

		// Medical rules
		medPairs := [][2]string{
			{`\bpatient percent with\b`, "patient presented with"},
			{`\bpost operative\b`, "postoperative"},
			{`\bpre operative\b`, "preoperative"},
			{`\bsub cutaneous\b`, "subcutaneous"},
			{`\bhyper tension\b`, "hypertension"},
			{`\bhypo tension\b`, "hypotension"},
			{`\bhyper thyroid\b`, "hyperthyroid"},
			{`\bhypo thyroid\b`, "hypothyroid"},
			{`\btachy cardia\b`, "tachycardia"},
			{`\bbrady cardia\b`, "bradycardia"},
			{`\belectro cardio gram\b`, "electrocardiogram"},
			{`\bin patient\b`, "inpatient"},
			{`\bout patient\b`, "outpatient"},
		}
		for _, pair := range medPairs {
			medicalRules = append(medicalRules, CorrectionRule{
				Pattern:     regexp.MustCompile(`(?i)` + pair[0]),
				Replacement: pair[1],
			})
		}

		// Legal rules
		legalPairs := [][2]string{
			{`\bper say\b`, "per se"},
			{`\bpro bow know\b`, "pro bono"},
			{`\baffy david\b`, "affidavit"},
			{`\bsub pina\b`, "subpoena"},
			{`\bsub poena\b`, "subpoena"},
			{`\bhave yes corpus\b`, "habeas corpus"},
			{`\bprima facy\b`, "prima facie"},
			{`\binter alia\b`, "inter alia"},
			{`\bquid pro crow\b`, "quid pro quo"},
		}
		for _, pair := range legalPairs {
			legalRules = append(legalRules, CorrectionRule{
				Pattern:     regexp.MustCompile(`(?i)` + pair[0]),
				Replacement: pair[1],
			})
		}
	})
}

// Corrector performs context-aware acoustic and homophone error correction.
type Corrector struct {
	mu          sync.RWMutex
	customRules map[string]string
}

// NewCorrector creates a Corrector with optional custom rules.
func NewCorrector(customRules map[string]string) *Corrector {
	initBuiltinRules()
	c := &Corrector{
		customRules: make(map[string]string),
	}
	if customRules != nil {
		for k, v := range customRules {
			c.customRules[strings.ToLower(strings.TrimSpace(k))] = v
		}
	}
	return c
}

func (c *Corrector) Name() string {
	return "corrector"
}

// Apply implements the postprocess.Step interface.
func (c *Corrector) Apply(text string, ctx Context) string {
	report := c.Correct(text, ctx.Application)
	return report.Corrected
}

// Correct runs self-correction on text given an active application or window title.
func (c *Corrector) Correct(text string, app string) CorrectionReport {
	initBuiltinRules()
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return CorrectionReport{Original: text, Corrected: text}
	}

	cat := appcontext.Categorize(app, "", "")
	current := text
	appliedCount := 0

	applyRules := func(rules []CorrectionRule) {
		for _, r := range rules {
			if r.Pattern.MatchString(current) {
				current = r.Pattern.ReplaceAllString(current, r.Replacement)
				appliedCount++
			}
		}
	}

	// 1. User-defined custom corrections (take highest precedence and override built-in rules)
	c.mu.RLock()
	for spoken, canonical := range c.customRules {
		if spoken == "" {
			continue
		}
		p := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(spoken) + `\b`)
		if p.MatchString(current) {
			current = p.ReplaceAllString(current, canonical)
			appliedCount++
		}
	}
	c.mu.RUnlock()

	// 2. Domain-specific corrections based on active application context
	switch cat {
	case appcontext.CategoryCode, appcontext.CategoryTerminal:
		applyRules(codeRules)
	case appcontext.CategoryMedical:
		applyRules(medicalRules)
	case appcontext.CategoryLegal:
		applyRules(legalRules)
	}

	// 3. General English acoustic corrections
	applyRules(generalRules)

	return CorrectionReport{
		Original:           text,
		Corrected:          current,
		App:                app,
		Category:           cat,
		CorrectionsApplied: appliedCount,
	}
}
