package manifest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/cail-health/cail-acquire/internal/manifest"
)

const withComment = `# header comment must survive
- id: nlpdp-sa-criteria
  url: https://www.gov.nl.ca/x/
  url_kind: index
  payload_confirmed: false
  last_payload_url: old.pdf
  raw_hash: UNVERIFIED
  normalized_hash: UNVERIFIED
  state: active
`

func TestUpdateEntry_WritesFieldsAndKeepsComment(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sources.yaml")
	if err := os.WriteFile(p, []byte(withComment), 0o644); err != nil {
		t.Fatal(err)
	}

	confirmed := true
	rh, nh, lp := "rawhash123", "normhash456", "https://www.gov.nl.ca/x/Criteria-July-2026.pdf"
	err := manifest.UpdateEntry(p, "nlpdp-sa-criteria", manifest.Update{
		PayloadConfirmed: &confirmed,
		RawHash:          &rh,
		NormalizedHash:   &nh,
		LastPayloadURL:   &lp,
	})
	if err != nil {
		t.Fatalf("UpdateEntry: %v", err)
	}

	raw, _ := os.ReadFile(p)
	if !strings.Contains(string(raw), "header comment must survive") {
		t.Error("header comment lost on rewrite")
	}

	m, err := manifest.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := m.Entry("nlpdp-sa-criteria")
	if !e.PayloadConfirmed || e.RawHash != rh || e.NormalizedHash != nh || e.LastPayloadURL != lp {
		t.Errorf("fields not updated: %+v", e)
	}
}

func TestUpdateEntry_WriteBoundary(t *testing.T) {
	dir := t.TempDir()
	deny := []string{
		filepath.Join(dir, "knowledge_base", "sources.yaml"),
		filepath.Join(dir, "DECISIONS.md"),
		filepath.Join(dir, "release.yaml"),
		filepath.Join(dir, "knowledge_base", "rules", "x.yaml"),
	}
	for _, p := range deny {
		if err := manifest.UpdateEntry(p, "x", manifest.Update{}); err == nil {
			t.Errorf("UpdateEntry(%q) allowed, want rejected by write boundary", p)
		}
	}
}
