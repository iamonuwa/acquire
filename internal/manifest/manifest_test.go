package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/cail-health/cail-acquire/internal/config"
	"gitlab.com/cail-health/cail-acquire/internal/manifest"
)

// Compile-time proof that *Manifest satisfies the interface config uses for
// the SPEC §4.1 cross-repo validation.
var _ config.ManifestIndex = (*manifest.Manifest)(nil)

const sample = `- id: nlpdp-sa-criteria
  jurisdiction: NL
  publisher: NL Health and Community Services
  url: https://www.gov.nl.ca/hcs/prescription/covered-specialauthdrugs/
  url_kind: index
  payload_confirmed: false
  raw_hash: UNVERIFIED
  normalized_hash: UNVERIFIED
  state: active
- id: hc-dpd-allfiles
  jurisdiction: CA
  publisher: Health Canada
  url: https://www.canada.ca/example.html
  url_kind: index
  state: active
`

func writeManifest(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "sources.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoad_IndexURLAndLookups(t *testing.T) {
	m, err := manifest.Load(writeManifest(t, sample))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	got, err := m.IndexURL("nlpdp-sa-criteria")
	if err != nil {
		t.Fatalf("IndexURL: %v", err)
	}
	want := "https://www.gov.nl.ca/hcs/prescription/covered-specialauthdrugs/"
	if got != want {
		t.Errorf("IndexURL = %q, want %q", got, want)
	}

	if !m.Has("hc-dpd-allfiles") || m.Has("nope") {
		t.Error("Has returned wrong membership")
	}
	if ids := m.ActiveIDs(); len(ids) != 2 {
		t.Errorf("ActiveIDs = %v, want 2 active", ids)
	}
}

func TestIndexURL_Failures(t *testing.T) {
	m, err := manifest.Load(writeManifest(t, sample))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.IndexURL("missing"); err == nil {
		t.Error("expected error for missing id")
	}

	badKind := `- id: x
  url: https://example.com/file.pdf
  url_kind: payload
  state: active`
	mb, err := manifest.Load(writeManifest(t, badKind))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mb.IndexURL("x"); err == nil {
		t.Error("expected error when url_kind != index")
	}
}

func TestLoad_DuplicateID(t *testing.T) {
	dup := `- id: same
  url: https://a
  url_kind: index
  state: active
- id: same
  url: https://b
  url_kind: index
  state: active`
	if _, err := manifest.Load(writeManifest(t, dup)); err == nil {
		t.Error("expected duplicate-id error")
	}
}

func TestLoad_Missing(t *testing.T) {
	if _, err := manifest.Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Error("expected error reading a missing manifest")
	}
}

// TestCrossValidation wires a real Manifest into config's §4.1 check.
func TestCrossValidation(t *testing.T) {
	m, err := manifest.Load(writeManifest(t, sample))
	if err != nil {
		t.Fatal(err)
	}
	tbl, err := config.Load([]byte(`sources:
  - id: nlpdp-sa-criteria
    jurisdiction: NL
    publisher: p
    fetch: scrape_anchor
    normalize: pdf
    scrape: {link_scope: 'a', anchor_pattern: 'x'}`))
	if err != nil {
		t.Fatal(err)
	}
	// nlpdp has a manifest entry, but hc-dpd-allfiles is active in the manifest
	// with no poll row → must be flagged.
	if err := tbl.ValidateAgainstManifest(m); err == nil {
		t.Error("expected orphan-active-entry error")
	}
}
