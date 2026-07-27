package config

import (
	"strings"
	"testing"
	"time"

	polldata "gitlab.com/cail-health/cail-acquire/config"
)

func TestLoad_EmbeddedTableIsValid(t *testing.T) {
	tbl, err := Load(polldata.PollYAML)
	if err != nil {
		t.Fatalf("embedded poll table failed to load: %v", err)
	}
	byID := map[string]*Source{}
	for i := range tbl.Sources {
		byID[tbl.Sources[i].ID] = &tbl.Sources[i]
	}
	nl := byID["nlpdp-sa-criteria"]
	if nl == nil {
		t.Fatal("nlpdp-sa-criteria missing from poll table")
	}
	if nl.Fetch != FetchScrapeAnchor || nl.Normalize != NormalizePDF {
		t.Errorf("nl row: got fetch=%q normalize=%q", nl.Fetch, nl.Normalize)
	}
	if !nl.Enabled {
		t.Error("nl row should be enabled")
	}
	if nl.Scrape.AnchorRE() == nil {
		t.Error("nl anchor pattern not compiled")
	}
	if dpd := byID["hc-dpd-allfiles"]; dpd == nil || dpd.Enabled {
		t.Error("dpd row should exist and be disabled until milestone 7")
	}
	if ab := byID["ab-idbl"]; ab == nil || !ab.IsUnresolved() {
		t.Error("ab-idbl should be an unresolved stub")
	}
}

const validNL = `
sources:
  - id: nlpdp-sa-criteria
    jurisdiction: NL
    publisher: NL Health and Community Services
    fetch: scrape_anchor
    normalize: pdf
    poll_interval: 24h
    staleness_alarm_days: 60
    enabled: true
    scrape:
      link_scope: 'a[href$=".pdf"]'
      anchor_pattern: 'Criteria-[A-Za-z]+-\d{4}\.pdf$'
    strip_rules: []
`

func TestLoad_ValidMinimal(t *testing.T) {
	if _, err := Load([]byte(validNL)); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
}

func TestValidate_Failures(t *testing.T) {
	cases := []struct {
		name   string
		yaml   string
		errSub string
	}{
		{
			name: "unknown fetch",
			yaml: `sources:
  - id: x
    jurisdiction: NL
    publisher: p
    fetch: wget
    normalize: pdf`,
			errSub: "unknown fetch",
		},
		{
			name: "unknown normalize",
			yaml: `sources:
  - id: x
    jurisdiction: NL
    publisher: p
    fetch: scrape_anchor
    normalize: ocr`,
			errSub: "unknown normalize",
		},
		{
			name: "bad anchor pattern",
			yaml: `sources:
  - id: x
    jurisdiction: NL
    publisher: p
    fetch: scrape_anchor
    normalize: pdf
    scrape:
      link_scope: 'a'
      anchor_pattern: '('`,
			errSub: "does not compile",
		},
		{
			name: "empty link_scope",
			yaml: `sources:
  - id: x
    jurisdiction: NL
    publisher: p
    fetch: scrape_anchor
    normalize: pdf
    scrape:
      anchor_pattern: 'x'`,
			errSub: "empty link_scope",
		},
		{
			name: "duplicate id",
			yaml: `sources:
  - id: dup
    jurisdiction: NL
    publisher: p
    fetch: scrape_anchor
    normalize: pdf
    scrape: {link_scope: 'a', anchor_pattern: 'x'}
  - id: dup
    state: unresolved`,
			errSub: "duplicate source id",
		},
		{
			name: "half-populated stub",
			yaml: `sources:
  - id: ab-idbl
    state: unresolved
    fetch: scrape_anchor`,
			errSub: "only id + state",
		},
		{
			name:   "missing id",
			yaml:   `sources:\n  - jurisdiction: NL`,
			errSub: "no id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load([]byte(strings.ReplaceAll(tc.yaml, `\n`, "\n")))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errSub)
			}
			if !strings.Contains(err.Error(), tc.errSub) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.errSub)
			}
		})
	}
}

func TestValidate_UnresolvedStubOK(t *testing.T) {
	y := `sources:
  - id: ab-idbl
    state: unresolved
  - id: qc-ramq
    state: unresolved`
	if _, err := Load([]byte(y)); err != nil {
		t.Fatalf("unresolved stubs rejected: %v", err)
	}
}

func TestDuration_Unmarshal(t *testing.T) {
	tbl, err := Load([]byte(validNL))
	if err != nil {
		t.Fatal(err)
	}
	if got := time.Duration(tbl.Sources[0].PollInterval); got != 24*time.Hour {
		t.Errorf("poll_interval: got %v want 24h", got)
	}

	bad := `sources:
  - id: x
    jurisdiction: NL
    publisher: p
    fetch: scrape_anchor
    normalize: pdf
    poll_interval: "not-a-duration"
    scrape: {link_scope: 'a', anchor_pattern: 'x'}`
	if _, err := Load([]byte(bad)); err == nil {
		t.Error("expected invalid poll_interval to fail")
	}
}

type fakeManifest struct {
	has    map[string]bool
	active []string
}

func (f fakeManifest) Has(id string) bool  { return f.has[id] }
func (f fakeManifest) ActiveIDs() []string { return f.active }

func TestValidateAgainstManifest(t *testing.T) {
	tbl, err := Load([]byte(validNL))
	if err != nil {
		t.Fatal(err)
	}

	ok := fakeManifest{has: map[string]bool{"nlpdp-sa-criteria": true}, active: []string{"nlpdp-sa-criteria"}}
	if err := tbl.ValidateAgainstManifest(ok); err != nil {
		t.Errorf("matching manifest rejected: %v", err)
	}

	missing := fakeManifest{has: map[string]bool{}, active: nil}
	if err := tbl.ValidateAgainstManifest(missing); err == nil {
		t.Error("expected error: poll row with no sources.yaml entry")
	}

	orphan := fakeManifest{
		has:    map[string]bool{"nlpdp-sa-criteria": true},
		active: []string{"nlpdp-sa-criteria", "hc-dpd-allfiles"},
	}
	if err := tbl.ValidateAgainstManifest(orphan); err == nil {
		t.Error("expected error: active sources.yaml entry with no poll row")
	}
}
