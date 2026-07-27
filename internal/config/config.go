// Package config loads and validates the poll table (SPEC §4.1).
//
// Two row shapes exist: full source rows and `state: unresolved` stubs
// (e.g. ab-idbl, qc-ramq) which carry only an id + state so CI counts them.
//
// All validation failures here are fatal and map to exit code 40 (SPEC §5.1):
// a bad config means nothing ran. The loader never degrades to a guess
// (CLAUDE rule 10).
package config

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// Fetch is the fetch-strategy enum. It is config data, not per-source code
// (SPEC §2, §6.1): adding a source is a config row, not a new code path.
type Fetch string

const (
	FetchScrapeAnchor Fetch = "scrape_anchor"
	FetchArchive      Fetch = "archive" // reserved for DPD at milestone 7
)

// Normalize is the normalizer enum.
type Normalize string

const (
	NormalizePDF        Normalize = "pdf"
	NormalizeZipMembers Normalize = "zip_members"
)

const stateUnresolved = "unresolved"

// Duration wraps time.Duration so YAML strings like "24h" / "168h" decode
// (yaml.v3 will not unmarshal into a bare time.Duration).
type Duration time.Duration

// UnmarshalYAML parses a Go duration string (e.g. "24h").
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return fmt.Errorf("poll_interval must be a duration string: %w", err)
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid poll_interval %q: %w", s, err)
	}
	*d = Duration(parsed)
	return nil
}

// StripRule normalizes a payload before hashing. Per CLAUDE rule 17 every rule
// carries a Note naming the observed false positive and its date; the list
// stays empty until the soak produces observations.
type StripRule struct {
	Pattern string `yaml:"pattern"`
	Note    string `yaml:"note"`
}

// Scrape is the scrape_anchor configuration for a source.
type Scrape struct {
	LinkScope     string `yaml:"link_scope"`     // CSS selector, e.g. a[href$=".pdf"]
	AnchorPattern string `yaml:"anchor_pattern"` // regex, e.g. Criteria-[A-Za-z]+-\d{4}\.pdf$

	anchorRE *regexp.Regexp // compiled at load, not serialized
}

// AnchorRE returns the compiled anchor pattern. It is nil until the row passes
// ValidateStandalone, which compiles it.
func (s *Scrape) AnchorRE() *regexp.Regexp { return s.anchorRE }

// Source is one poll-table row. A stub carries only ID + State; a full row
// carries the rest.
type Source struct {
	ID    string `yaml:"id"`
	State string `yaml:"state"` // "" (full row) or "unresolved" (stub)

	Jurisdiction       string      `yaml:"jurisdiction"`
	Publisher          string      `yaml:"publisher"`
	Fetch              Fetch       `yaml:"fetch"`
	Normalize          Normalize   `yaml:"normalize"`
	PollInterval       Duration    `yaml:"poll_interval"`
	StalenessAlarmDays int         `yaml:"staleness_alarm_days"`
	Enabled            bool        `yaml:"enabled"`
	Scrape             Scrape      `yaml:"scrape"`
	StripRules         []StripRule `yaml:"strip_rules"`
}

// IsUnresolved reports whether the row is an unresolved stub.
func (s *Source) IsUnresolved() bool { return s.State == stateUnresolved }

// PollTable is the full parsed config.
type PollTable struct {
	Sources []Source `yaml:"sources"`
}

// Load parses raw YAML and runs ValidateStandalone. A returned error is fatal
// (exit 40).
func Load(raw []byte) (*PollTable, error) {
	var t PollTable
	if err := yaml.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("config: parse poll table: %w", err)
	}
	if err := t.ValidateStandalone(); err != nil {
		return nil, err
	}
	return &t, nil
}

// ValidateStandalone runs every check that does NOT require the cail-rules
// working copy. All failures are fatal (SPEC §4.1, §5.1). It also compiles each
// scrape_anchor row's anchor pattern in place.
func (t *PollTable) ValidateStandalone() error {
	seen := make(map[string]bool, len(t.Sources))
	for i := range t.Sources {
		s := &t.Sources[i]

		if s.ID == "" {
			return fmt.Errorf("config: source #%d has no id", i)
		}
		if seen[s.ID] {
			return fmt.Errorf("config: duplicate source id %q", s.ID)
		}
		seen[s.ID] = true

		if s.IsUnresolved() {
			if err := validateStub(s); err != nil {
				return err
			}
			continue
		}
		if err := validateFullRow(s); err != nil {
			return err
		}
	}
	return nil
}

// validateStub enforces that an unresolved row carries nothing but id + state.
// A half-populated stub is a mistake, not a poll target (SPEC §4.1).
func validateStub(s *Source) error {
	if s.Fetch != "" || s.Normalize != "" || s.Enabled ||
		s.PollInterval != 0 || s.Scrape.LinkScope != "" || s.Scrape.AnchorPattern != "" {
		return fmt.Errorf("config: unresolved source %q must carry only id + state", s.ID)
	}
	return nil
}

func validateFullRow(s *Source) error {
	if s.Jurisdiction == "" || s.Publisher == "" {
		return fmt.Errorf("config: source %q missing jurisdiction or publisher", s.ID)
	}
	switch s.Fetch {
	case FetchScrapeAnchor, FetchArchive:
	case "":
		return fmt.Errorf("config: source %q has no fetch strategy", s.ID)
	default:
		return fmt.Errorf("config: source %q unknown fetch %q", s.ID, s.Fetch)
	}
	switch s.Normalize {
	case NormalizePDF, NormalizeZipMembers:
	case "":
		return fmt.Errorf("config: source %q has no normalizer", s.ID)
	default:
		return fmt.Errorf("config: source %q unknown normalize %q", s.ID, s.Normalize)
	}
	if s.Fetch == FetchScrapeAnchor {
		if s.Scrape.LinkScope == "" {
			return fmt.Errorf("config: scrape_anchor source %q has empty link_scope", s.ID)
		}
		if s.Scrape.AnchorPattern == "" {
			return fmt.Errorf("config: scrape_anchor source %q has empty anchor_pattern", s.ID)
		}
		re, err := regexp.Compile(s.Scrape.AnchorPattern)
		if err != nil {
			return fmt.Errorf("config: source %q anchor_pattern does not compile: %w", s.ID, err)
		}
		s.Scrape.anchorRE = re
	}
	return nil
}

// ManifestIndex is the read view of sources.yaml (in the cail-rules working
// copy) needed for cross-repo validation. internal/manifest implements it.
type ManifestIndex interface {
	// Has reports whether sources.yaml has an entry for id.
	Has(id string) bool
	// ActiveIDs lists sources.yaml entries in state: active.
	ActiveIDs() []string
}

// ValidateAgainstManifest enforces the two cross-repo rules in SPEC §4.1:
// a full poll row with no matching sources.yaml entry, and a sources.yaml
// state:active entry with no poll row. It runs only once a manifest is loaded.
func (t *PollTable) ValidateAgainstManifest(m ManifestIndex) error {
	if m == nil {
		return errors.New("config: nil manifest index")
	}
	pollIDs := make(map[string]bool, len(t.Sources))
	for i := range t.Sources {
		s := &t.Sources[i]
		pollIDs[s.ID] = true
		if s.IsUnresolved() {
			continue
		}
		if !m.Has(s.ID) {
			return fmt.Errorf("config: poll row %q has no sources.yaml entry", s.ID)
		}
	}
	for _, id := range m.ActiveIDs() {
		if !pollIDs[id] {
			return fmt.Errorf("config: sources.yaml active entry %q has no poll row", id)
		}
	}
	return nil
}
