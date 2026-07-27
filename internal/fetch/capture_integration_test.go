//go:build integration

package fetch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// nlIndexURL is the verified NL index (manifest) URL (SPEC §3.1). It is the
// stable index page, not the date-stamped payload URL — capturing it here is
// fine (CLAUDE rule 2 forbids hardcoding the *payload* URL).
const nlIndexURL = "https://www.gov.nl.ca/hcs/prescription/covered-specialauthdrugs/"

// TestCaptureNLIndex fetches the live NL index page and writes it beside the
// synthetic fixtures as *_live.html for a human to review before commit
// (CLAUDE rule 13). Run via `make regen-fixtures`. It never overwrites the
// deterministic synthetic fixtures. It also asserts the live page still
// resolves to exactly one Criteria PDF, so a page-shape change fails loudly.
func TestCaptureNLIndex(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	c := NewClient()
	body, _, err := c.Get(ctx, nlIndexURL)
	if err != nil {
		t.Fatalf("live fetch of NL index failed: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("live NL index returned an empty body")
	}

	out := filepath.Join("..", "..", "testdata", "nlpdp_index_happy_live.html")
	if err := os.WriteFile(out, body, 0o644); err != nil {
		t.Fatalf("write captured fixture: %v", err)
	}
	t.Logf("captured %d bytes to %s — review before commit (CLAUDE rule 13)", len(body), out)

	m, err := ScrapeAnchor{}.Resolve(ctx, c, nlIndexURL, nlSpec())
	if err != nil {
		t.Fatalf("live index no longer resolves to exactly one Criteria PDF: %v", err)
	}
	t.Logf("resolved payload URL: %s (anchor: %q)", m.URL, m.AnchorText)
}
