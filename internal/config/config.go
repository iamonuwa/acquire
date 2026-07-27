// Package config loads and validates the poll table.
package config

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// Fetch is the fetch-strategy enum.
type Fetch string

const (
	// FetchScrapeAnchor discovers the payload URL by scraping the index page.
	FetchScrapeAnchor Fetch = "scrape_anchor"
	// FetchArchive downloads and extracts an archive (not built yet).
	FetchArchive Fetch = "archive"
)

// Normalize is the normalizer enum.
type Normalize string

const (
	// NormalizePDF extracts text with pdftotext -layout.
	NormalizePDF Normalize = "pdf"
	// NormalizeZipMembers concatenates ZIP member text (not built yet).
	NormalizeZipMembers Normalize = "zip_members"
)

const stateUnresolved = "unresolved"

// Duration decodes YAML duration strings like "24h".
type Duration time.Duration

// UnmarshalYAML decodes a Go duration string such as "24h".
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

// StripRule normalizes a payload before hashing. Note names the observed false
// positive and its date (empty until the soak).
type StripRule struct {
	Pattern string `yaml:"pattern"`
	Note    string `yaml:"note"`
}

// Scrape is a source's scrape_anchor configuration.
type Scrape struct {
	LinkScope     string `yaml:"link_scope"`
	AnchorPattern string `yaml:"anchor_pattern"`

	anchorRE *regexp.Regexp
}

// AnchorRE returns the compiled anchor pattern; nil until ValidateStandalone runs.
func (s *Scrape) AnchorRE() *regexp.Regexp { return s.anchorRE }

// Source is one poll-table row. A stub carries only ID + State.
type Source struct {
	ID    string `yaml:"id"`
	State string `yaml:"state"`

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

// PollTable is the parsed poll table.
type PollTable struct {
	Sources []Source `yaml:"sources"`
}

// Load parses raw YAML and runs ValidateStandalone. Errors are fatal (exit 40).
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

// ValidateStandalone runs every check not needing the cail-rules working copy,
// compiling each scrape_anchor row's pattern in place. All failures are fatal.
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

// ManifestIndex is the read view of sources.yaml needed for cross-repo validation.
type ManifestIndex interface {
	Has(id string) bool
	ActiveIDs() []string
}

// ValidateAgainstManifest flags a poll row with no sources.yaml entry and an
// active sources.yaml entry with no poll row.
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
