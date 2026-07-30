package phigate

import (
	"strings"
	"testing"
)

func TestLuhn(t *testing.T) {
	if !luhnValid([]byte("046454286")) { // synthetic, Luhn-valid
		t.Error("expected 046454286 to pass Luhn")
	}
	if luhnValid([]byte("046454287")) { // one digit off
		t.Error("expected 046454287 to fail Luhn")
	}
}

func TestCheck_Hits(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string // substring expected in Reason
	}{
		{"SIN luhn-valid", "file 046 454 286 today", "SIN"},
		{"SIN packed", "046454286", "SIN"},
		{"MCP 12 digits", "card 123456789012 x", "NL-MCP"},
		{"MCP spaced", "card 1234 5678 9012 x", "NL-MCP"},
		{"phone near name", "John Smith 709-555-1234", "phone"},
		{"postal near name", "Jane Roe A1B 2C3", "postal"},
		{"dob near name", "Jane Roe 1980-05-15", "DOB"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Check([]byte(tc.text))
			if !res.Hit {
				t.Fatalf("expected a hit, got none")
			}
			if !strings.Contains(res.Reason, tc.want) {
				t.Errorf("reason %q missing %q", res.Reason, tc.want)
			}
			// Reason must never echo the matched value.
			for _, leak := range []string{"046454286", "123456789012", "709-555-1234", "A1B", "1980-05-15"} {
				if strings.Contains(res.Reason, leak) {
					t.Errorf("reason leaked matched value: %q", res.Reason)
				}
			}
		})
	}
}

func TestCheck_Clean(t *testing.T) {
	clean := []string{
		"Metformin 500 mg tablet",
		"DIN 02422425 covered under special authorization", // 8-digit DIN, not SIN/MCP
		"046 454 287 is not a valid checksum",              // Luhn-invalid 9-digit run
		"Updated July 16, 2026",                            // date, no ISO/name context
		"call 709-555-1234 for the program",                // phone with no name nearby
		"unit A1B 2C3 warehouse",                           // postal with no name nearby
		"DELSTRIGO 100-300-300 MG Tablet",                  // drug strength, not a SIN
		"dose 046-454-286 mg twice daily",                  // Luhn-valid shape but a dosage
	}
	for _, s := range clean {
		if res := Check([]byte(s)); res.Hit {
			t.Errorf("false positive on %q: %s", s, res.Reason)
		}
	}
}
