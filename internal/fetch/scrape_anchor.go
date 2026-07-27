package fetch

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"gitlab.com/cail-health/cail-acquire/internal/config"
)

// AnchorMatch is hop-1's result. AnchorText (e.g. "Last updated on ...") is
// recorded for the diff header, never hashed.
type AnchorMatch struct {
	URL        *url.URL
	RawHref    string
	AnchorText string
}

// ScrapeSpec is the fetch-relevant projection of a source's scrape config.
type ScrapeSpec struct {
	LinkScope string
	AnchorRE  *regexp.Regexp
}

// SpecFor builds a ScrapeSpec from a validated source row.
func SpecFor(s *config.Source) ScrapeSpec {
	return ScrapeSpec{LinkScope: s.Scrape.LinkScope, AnchorRE: s.Scrape.AnchorRE()}
}

// Strategy is a fetch enum's behavior.
type Strategy interface {
	Resolve(ctx context.Context, c *Client, indexURL string, spec ScrapeSpec) (*AnchorMatch, error)
}

// ScrapeAnchor implements the scrape_anchor strategy (hop 1 here).
type ScrapeAnchor struct{}

var registry = map[config.Fetch]Strategy{
	config.FetchScrapeAnchor: ScrapeAnchor{},
}

// Get returns the fetch strategy for f, or a fatal error if none is registered.
func Get(f config.Fetch) (Strategy, error) {
	s, ok := registry[f]
	if !ok {
		return nil, fmt.Errorf("fetch: no strategy for %q (unknown or not yet implemented)", f)
	}
	return s, nil
}

// Resolve GETs the index page and requires exactly one in-scope anchor matching
// spec.AnchorRE. Zero and two-or-more are both failures, never pick-the-first.
func (ScrapeAnchor) Resolve(ctx context.Context, c *Client, indexURL string, spec ScrapeSpec) (*AnchorMatch, error) {
	if spec.AnchorRE == nil {
		return nil, fmt.Errorf("fetch: scrape_anchor for %s has no compiled anchor pattern", indexURL)
	}
	body, finalURL, err := c.Get(ctx, indexURL)
	if err != nil {
		return nil, err
	}

	base := finalURL
	if base == nil {
		if base, err = url.Parse(indexURL); err != nil {
			return nil, fmt.Errorf("fetch: parse index URL %s: %w", indexURL, err)
		}
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("fetch: parse index page %s: %w", indexURL, err)
	}

	var matches []*AnchorMatch
	seen := make(map[string]bool)
	var scanned int
	var selErr error

	doc.Find(spec.LinkScope).EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		href, ok := sel.Attr("href")
		if !ok {
			return true
		}
		scanned++
		if !spec.AnchorRE.MatchString(href) {
			return true
		}
		ref, perr := url.Parse(href)
		if perr != nil {
			selErr = fmt.Errorf("fetch: matched href %q does not parse: %w", href, perr)
			return false
		}
		abs := base.ResolveReference(ref)
		key := abs.String()
		if seen[key] {
			return true
		}
		seen[key] = true
		matches = append(matches, &AnchorMatch{
			URL:        abs,
			RawHref:    href,
			AnchorText: collapseSpace(sel.Text()),
		})
		return true
	})
	if selErr != nil {
		return nil, selErr
	}

	switch len(matches) {
	case 0:
		return nil, &NoMatchError{
			IndexURL:  indexURL,
			LinkScope: spec.LinkScope,
			Pattern:   spec.AnchorRE.String(),
			Scanned:   scanned,
		}
	case 1:
		return matches[0], nil
	default:
		urls := make([]string, len(matches))
		for i, m := range matches {
			urls[i] = m.URL.String()
		}
		return nil, &MultiMatchError{
			IndexURL:  indexURL,
			LinkScope: spec.LinkScope,
			Pattern:   spec.AnchorRE.String(),
			Matches:   urls,
		}
	}
}

// NoMatchError means zero in-scope links matched. Scanned distinguishes a
// selector miss from a regex miss.
type NoMatchError struct {
	IndexURL  string
	LinkScope string
	Pattern   string
	Scanned   int
}

func (e *NoMatchError) Error() string {
	return fmt.Sprintf("scrape_anchor: 0 links matched /%s/ within %q at %s (%d in-scope candidates scanned)",
		e.Pattern, e.LinkScope, e.IndexURL, e.Scanned)
}

// MultiMatchError means two or more distinct links matched; it lists them all.
type MultiMatchError struct {
	IndexURL  string
	LinkScope string
	Pattern   string
	Matches   []string
}

func (e *MultiMatchError) Error() string {
	return fmt.Sprintf("scrape_anchor: %d links matched /%s/ at %s (ambiguous, not resolved): %s",
		len(e.Matches), e.Pattern, e.IndexURL, strings.Join(e.Matches, ", "))
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
