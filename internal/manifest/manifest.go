// Package manifest reads sources.yaml from the cail-rules working copy.
package manifest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	stateActive  = "active"
	urlKindIndex = "index"
)

// Entry is one sources.yaml record.
type Entry struct {
	ID               string `yaml:"id"`
	Jurisdiction     string `yaml:"jurisdiction"`
	Publisher        string `yaml:"publisher"`
	URL              string `yaml:"url"`
	URLKind          string `yaml:"url_kind"`
	PayloadConfirmed bool   `yaml:"payload_confirmed"`
	LastPayloadURL   string `yaml:"last_payload_url"`
	Licence          string `yaml:"licence"`
	RawHash          string `yaml:"raw_hash"`
	NormalizedHash   string `yaml:"normalized_hash"`
	State            string `yaml:"state"`
}

// Manifest is the parsed sources.yaml.
type Manifest struct {
	entries []Entry
	byID    map[string]*Entry
}

// Load reads and parses sources.yaml (a top-level YAML sequence) from path.
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

// IndexURL returns the index URL for id, erroring if absent, empty, or not an index url.
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

// Has reports whether an entry for id exists.
func (m *Manifest) Has(id string) bool {
	_, ok := m.byID[id]
	return ok
}

// ActiveIDs lists entries in state: active.
func (m *Manifest) ActiveIDs() []string {
	var ids []string
	for i := range m.entries {
		if m.entries[i].State == stateActive {
			ids = append(ids, m.entries[i].ID)
		}
	}
	return ids
}
