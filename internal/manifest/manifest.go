// Package manifest reads sources.yaml from the cail-rules working copy
// (SPEC §2, §4.2). The binary holds no manifest state of its own.
//
// Milestone 2 needs only the read side: the index URL for a source, and the
// two lookups (Has, ActiveIDs) that satisfy config.ManifestIndex for the §4.1
// cross-repo validation. Writing hash/provenance fields back is milestone 3+.
package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const stateActive = "active"

// urlKindIndex is the only url_kind the fetcher treats as an index page.
const urlKindIndex = "index"

// Entry is one sources.yaml record. Only the fields milestone 2 reads are
// modelled explicitly; the rest are tolerated by yaml's default of ignoring
// unknown keys.
type Entry struct {
	ID             string `yaml:"id"`
	Jurisdiction   string `yaml:"jurisdiction"`
	Publisher      string `yaml:"publisher"`
	URL            string `yaml:"url"`
	URLKind        string `yaml:"url_kind"`
	LastPayloadURL string `yaml:"last_payload_url"`
	Licence        string `yaml:"licence"`
	RawHash        string `yaml:"raw_hash"`
	NormalizedHash string `yaml:"normalized_hash"`
	State          string `yaml:"state"`
}

// Manifest is the parsed sources.yaml.
type Manifest struct {
	entries []Entry
	byID    map[string]*Entry
}

// Load reads and parses sources.yaml from path. sources.yaml is a top-level
// YAML sequence of entries (SPEC §4.2), not a mapping.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("manifest: read %s: %w", path, err)
	}
	var entries []Entry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("manifest: parse %s: %w", path, err)
	}
	m := &Manifest{entries: entries, byID: make(map[string]*Entry, len(entries))}
	for i := range m.entries {
		e := &m.entries[i]
		if e.ID == "" {
			return nil, fmt.Errorf("manifest: %s has an entry with no id", path)
		}
		if m.byID[e.ID] != nil {
			return nil, fmt.Errorf("manifest: %s has duplicate id %q", path, e.ID)
		}
		m.byID[e.ID] = e
	}
	return m, nil
}

// Entry returns the entry for id.
func (m *Manifest) Entry(id string) (*Entry, bool) {
	e, ok := m.byID[id]
	return e, ok
}

// IndexURL returns the index-page URL for id — the input hop 1 fetches. It is
// an error if the source is absent, has no url, or is not url_kind: index
// (fail loudly rather than fetch the wrong thing — CLAUDE rule 10).
func (m *Manifest) IndexURL(id string) (string, error) {
	e, ok := m.byID[id]
	if !ok {
		return "", fmt.Errorf("manifest: no entry for %q", id)
	}
	if e.URL == "" {
		return "", fmt.Errorf("manifest: entry %q has no url", id)
	}
	if e.URLKind != urlKindIndex {
		return "", fmt.Errorf("manifest: entry %q url_kind is %q, want %q", id, e.URLKind, urlKindIndex)
	}
	return e.URL, nil
}

// Has reports whether sources.yaml has an entry for id (config.ManifestIndex).
func (m *Manifest) Has(id string) bool {
	_, ok := m.byID[id]
	return ok
}

// ActiveIDs lists entries in state: active (config.ManifestIndex).
func (m *Manifest) ActiveIDs() []string {
	var ids []string
	for i := range m.entries {
		if m.entries[i].State == stateActive {
			ids = append(ids, m.entries[i].ID)
		}
	}
	return ids
}
