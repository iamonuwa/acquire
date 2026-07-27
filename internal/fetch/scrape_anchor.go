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

// AnchorMatch is hop-1's result: the discovered payload URL plus the context a
// human reviewer reads in the diff header (SPEC §6.1). AnchorText carries e.g.
// NL's "Last updated on July 16, 2026" — recorded, never hashed (CLAUDE.md).
type AnchorMatch struct {
	URL        *url.URL // absolute, resolved against the index page's URL
	RawHref    string   // href exactly as written in the page
	AnchorText string   // whitespace-collapsed text of the matched anchor
}

// ScrapeSpec is the fetch-relevant projection of a poll row's scrape config.
type ScrapeSpec struct {
	LinkScope string         // CSS selector, e.g. a[href$=".pdf"]
	AnchorRE  *regexp.Regexp // compiled anchor pattern
}

// SpecFor builds a ScrapeSpec from a validated source row.
func SpecFor(s *config.Source) ScrapeSpec {
	return ScrapeSpec{LinkScope: s.Scrape.LinkScope, AnchorRE: s.Scrape.AnchorRE()}
}

// Strategy is the fetch enum's behavior, not per-source code (SPEC §2, §6.1).
// DPD (milestone 7) adds one implementation and one registry entry, nothing else.
type Strategy interface {
	// Resolve is hop 1: discover the single payload URL from the index page.
	// (Hop 2, the payload GET, is added at milestone 3.)
	Resolve(ctx context.Context, c *Client, indexURL string, spec ScrapeSpec) (*AnchorMatch, error)
}

// ScrapeAnchor implements the two-hop scrape_anchor strategy (hop 1 here).
type ScrapeAnchor struct{}

var registry = map[config.Fetch]Strategy{
	config.FetchScrapeAnchor: ScrapeAnchor{},
	// config.FetchArchive is registered at milestone 7.
}

// Get returns the fetch strategy for a config enum value. An unknown or
// not-yet-implemented strategy (e.g. archive before milestone 7) is a fatal
// config error (exit 40) — never a silent fallback (CLAUDE rule 10).
func Get(f config.Fetch) (Strategy, error) {
	s, ok := registry[f]
	if !ok {
		return nil, fmt.Errorf("fetch: no strategy for %q (unknown or not yet implemented)", f)
	}
	return s, nil
}

// Resolve GETs the index page and requires exactly one anchor whose href
// matches spec.AnchorRE within spec.LinkScope. Zero matches and two-or-more
// matches are BOTH failures — never pick-the-first (SPEC §6.1, CLAUDE rule 10).
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
			return true // same link listed twice is not a multi-match
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

// NoMatchError means zero in-scope links matched the anchor pattern. Scanned
// distinguishes "selector matched nothing" from "regex matched nothing".
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

// MultiMatchError means two or more distinct links matched. It lists them all
// so the ambiguity is loud — never resolved by picking the first (SPEC §6.1).
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

// collapseSpace trims and collapses internal runs of whitespace to single
// spaces, so multi-line anchor text records as one clean line.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
