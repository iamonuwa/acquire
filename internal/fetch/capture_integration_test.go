//go:build integration

package fetch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// nlIndexURL is the verified NL index (manifest) URL, not the payload URL.
const nlIndexURL = "https://www.gov.nl.ca/hcs/prescription/covered-specialauthdrugs/"

// TestCaptureNLIndex fetches the live NL index into a *_live.html for human
// review (run via `make regen-fixtures`) and asserts it still resolves to
// exactly one Criteria PDF.
func TestCaptureNLIndex(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	c := NewClient()
	resp, err := c.Fetch(ctx, nlIndexURL)
	if err != nil {
		t.Fatalf("live fetch of NL index failed: %v", err)
	}
	if len(resp.Body) == 0 {
		t.Fatal("live NL index returned an empty body")
	}

	out := filepath.Join("..", "..", "testdata", "nlpdp_index_happy_live.html")
	if err := os.WriteFile(out, resp.Body, 0o644); err != nil {
		t.Fatalf("write captured fixture: %v", err)
	}
	t.Logf("captured %d bytes to %s — review before commit", len(resp.Body), out)

	m, err := ScrapeAnchor{}.Resolve(ctx, c, nlIndexURL, nlSpec())
	if err != nil {
		t.Fatalf("live index no longer resolves to exactly one Criteria PDF: %v", err)
	}
	t.Logf("resolved payload URL: %s (anchor: %q)", m.URL, m.AnchorText)
}
