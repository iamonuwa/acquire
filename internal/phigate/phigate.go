// Package phigate is the PHI hard stop that fires before any write.
package phigate

import (
	"fmt"
	"regexp"
	"strings"
)

// Result is the outcome of a PHI scan. Reason names the patterns and their
// offsets; it never includes the matched text, which could itself be PHI.
type Result struct {
	Hit    bool
	Reason string
}

var (
	sinRE    = regexp.MustCompile(`\b\d{3}[-\s]?\d{3}[-\s]?\d{3}\b`)
	mcpRE    = regexp.MustCompile(`\b\d{12}\b`)
	mcpSpRE  = regexp.MustCompile(`\b\d{4}[-\s]\d{4}[-\s]\d{4}\b`)
	phoneRE  = regexp.MustCompile(`\b\d{3}[-.\s]\d{3}[-.\s]\d{4}\b`)
	postalRE = regexp.MustCompile(`\b[A-Za-z]\d[A-Za-z][-\s]?\d[A-Za-z]\d\b`)
	dobRE    = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	nameRE   = regexp.MustCompile(`\b[A-Z][a-z]+ [A-Z][a-z]+\b`)

	// A dosage unit right after a 9-digit run marks it as a drug strength, not
	// a SIN. Observed false positive 2026-07-30: "DELSTRIGO 100-300-300 MG
	// Tablet" in the NL criteria PDF. A SIN is never followed by a dosage.
	dosageUnitRE = regexp.MustCompile(`(?i)^[\s(]*(mg|mcg|ml|g|%|iu|units?|tablet|capsule|caps?|tabs?)\b`)
)

// nameWindow is how many bytes around a supporting match to search for a
// person-name-shaped token before treating it as PHI.
const nameWindow = 40

type supporting struct {
	name string
	re   *regexp.Regexp
}

// Supporting signals only fire next to a name, which alone are common in any
// document; SIN and NL MCP fire on their own.
var supportingPatterns = []supporting{
	{"phone", phoneRE},
	{"postal", postalRE},
	{"DOB", dobRE},
}

// Check scans normalized text for PHI, tuned to over-fire: a false positive
// costs one alert, a false negative puts patient data in a permanent object.
// Any hit halts the source and writes nothing.
func Check(text []byte) Result {
	var hits []string

	for _, loc := range sinRE.FindAllIndex(text, -1) {
		if !luhnValid(onlyDigits(text[loc[0]:loc[1]])) {
			continue
		}
		if isDosage(text, loc[1]) {
			continue
		}
		hits = append(hits, fmt.Sprintf("SIN@%d", loc[0]))
	}
	for _, re := range []*regexp.Regexp{mcpRE, mcpSpRE} {
		for _, loc := range re.FindAllIndex(text, -1) {
			hits = append(hits, fmt.Sprintf("NL-MCP@%d", loc[0]))
		}
	}
	for _, s := range supportingPatterns {
		for _, loc := range s.re.FindAllIndex(text, -1) {
			if nameNear(text, loc[0], loc[1]) {
				hits = append(hits, fmt.Sprintf("%s@%d", s.name, loc[0]))
			}
		}
	}

	if len(hits) == 0 {
		return Result{}
	}
	return Result{Hit: true, Reason: strings.Join(hits, ", ")}
}

// isDosage reports whether the run ending at end is immediately followed by a
// drug-dosage unit, which marks it as a strength rather than a SIN.
func isDosage(text []byte, end int) bool {
	hi := end + 20
	if hi > len(text) {
		hi = len(text)
	}
	return dosageUnitRE.Match(text[end:hi])
}

func nameNear(text []byte, start, end int) bool {
	lo := start - nameWindow
	if lo < 0 {
		lo = 0
	}
	hi := end + nameWindow
	if hi > len(text) {
		hi = len(text)
	}
	return nameRE.Match(text[lo:hi])
}

func onlyDigits(b []byte) []byte {
	var out []byte
	for _, c := range b {
		if c >= '0' && c <= '9' {
			out = append(out, c)
		}
	}
	return out
}

// luhnValid reports whether digits pass the Luhn mod-10 check.
func luhnValid(digits []byte) bool {
	if len(digits) == 0 {
		return false
	}
	sum, double := 0, false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}
