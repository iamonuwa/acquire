package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// nlSpec is the NL scrape configuration (SPEC §4.1): select .pdf links, keep
// the Criteria-<Month>-<Year>.pdf one.
func nlSpec() ScrapeSpec {
	return ScrapeSpec{
		LinkScope: `a[href$=".pdf"]`,
		AnchorRE:  regexp.MustCompile(`Criteria-[A-Za-z]+-\d{4}\.pdf$`),
	}
}

// serveFixture serves the named testdata file for every path.
func serveFixture(t *testing.T, name string) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(data)
	}))
}

func TestResolveAnchor_Happy(t *testing.T) {
	srv := serveFixture(t, "nlpdp_index_happy.html")
	defer srv.Close()

	m, err := ScrapeAnchor{}.Resolve(context.Background(), NewClient(), srv.URL, nlSpec())
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if got := filepath.Base(m.URL.Path); got != "Criteria-July-2026.pdf" {
		t.Errorf("resolved filename = %q, want Criteria-July-2026.pdf", got)
	}
	if m.URL.Path != "/hcs/files/Criteria-July-2026.pdf" {
		t.Errorf("resolved path = %q, want /hcs/files/Criteria-July-2026.pdf", m.URL.Path)
	}
	if !strings.Contains(m.AnchorText, "Last updated on July 16, 2026") {
		t.Errorf("anchor text %q missing 'Last updated on' date", m.AnchorText)
	}
}

func TestResolveAnchor_ZeroMatch(t *testing.T) {
	srv := serveFixture(t, "nlpdp_index_zero.html")
	defer srv.Close()

	_, err := ScrapeAnchor{}.Resolve(context.Background(), NewClient(), srv.URL, nlSpec())
	var nm *NoMatchError
	if !errors.As(err, &nm) {
		t.Fatalf("expected *NoMatchError, got %v", err)
	}
	if nm.Scanned != 2 {
		t.Errorf("Scanned = %d, want 2 (selector matched, regex filtered all)", nm.Scanned)
	}
}

func TestResolveAnchor_MultiMatch(t *testing.T) {
	srv := serveFixture(t, "nlpdp_index_multi.html")
	defer srv.Close()

	_, err := ScrapeAnchor{}.Resolve(context.Background(), NewClient(), srv.URL, nlSpec())
	var mm *MultiMatchError
	if !errors.As(err, &mm) {
		t.Fatalf("expected *MultiMatchError, got %v", err)
	}
	if len(mm.Matches) != 2 {
		t.Fatalf("Matches = %v, want 2", mm.Matches)
	}
	joined := strings.Join(mm.Matches, " ")
	if !strings.Contains(joined, "Criteria-July-2026.pdf") || !strings.Contains(joined, "Criteria-June-2026.pdf") {
		t.Errorf("both matches should be listed (not pick-first): %v", mm.Matches)
	}
}

func TestResolveAnchor_RelativeResolution(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "nlpdp_index_relative.html"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	// Request from the real index sub-path so ../../ resolution is exercised.
	indexURL := srv.URL + "/hcs/prescription/covered-specialauthdrugs/"
	m, err := ScrapeAnchor{}.Resolve(context.Background(), NewClient(), indexURL, nlSpec())
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if m.URL.Path != "/hcs/files/Criteria-July-2026.pdf" {
		t.Errorf("relative href resolved to %q, want /hcs/files/Criteria-July-2026.pdf", m.URL.Path)
	}
}

// TestResolveAnchor_LinkScopeFilters proves the match is driven by the href
// (via link_scope + regex), not by anchor text: a link whose *text* is a
// Criteria filename but whose href is not a .pdf must be ignored.
func TestResolveAnchor_LinkScopeFilters(t *testing.T) {
	const page = `<!DOCTYPE html><html><body>
	  <a href="/notes.html">Criteria-July-2026.pdf</a>
	  <a href="/hcs/forms/SA-Request-Form.pdf">Request Form</a>
	</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(page))
	}))
	defer srv.Close()

	_, err := ScrapeAnchor{}.Resolve(context.Background(), NewClient(), srv.URL, nlSpec())
	var nm *NoMatchError
	if !errors.As(err, &nm) {
		t.Fatalf("expected *NoMatchError (href-driven match), got %v", err)
	}
	if nm.Scanned != 1 { // only the .pdf form link is in scope; the .html link is not
		t.Errorf("Scanned = %d, want 1", nm.Scanned)
	}
}
