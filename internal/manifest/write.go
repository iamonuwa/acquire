package manifest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Update carries the provenance/hash fields the acquirer may write back.
type Update struct {
	PayloadConfirmed *bool
	RawHash          *string
	NormalizedHash   *string
	LastPayloadURL   *string
}

// UpdateEntry rewrites only the given fields of one entry in sources.yaml,
// preserving comments and the rest of the file. It enforces the write boundary.
func UpdateEntry(path, id string, u Update) error {
	if err := guardWritePath(path); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("manifest: read %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("manifest: parse %s: %w", path, err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.SequenceNode {
		return fmt.Errorf("manifest: %s is not a top-level sequence", path)
	}

	var target *yaml.Node
	for _, item := range doc.Content[0].Content {
		if item.Kind == yaml.MappingNode && mappingValue(item, "id") == id {
			target = item
			break
		}
	}
	if target == nil {
		return fmt.Errorf("manifest: no entry %q in %s", id, path)
	}

	if u.PayloadConfirmed != nil {
		setScalar(target, "payload_confirmed", boolStr(*u.PayloadConfirmed), "!!bool")
	}
	if u.RawHash != nil {
		setScalar(target, "raw_hash", *u.RawHash, "!!str")
	}
	if u.NormalizedHash != nil {
		setScalar(target, "normalized_hash", *u.NormalizedHash, "!!str")
	}
	if u.LastPayloadURL != nil {
		setScalar(target, "last_payload_url", *u.LastPayloadURL, "!!str")
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("manifest: encode %s: %w", path, err)
	}
	_ = enc.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("manifest: write %s: %w", path, err)
	}
	return nil
}

func mappingValue(m *yaml.Node, key string) string {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1].Value
		}
	}
	return ""
}

func setScalar(m *yaml.Node, key, value, tag string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			v := m.Content[i+1]
			v.Kind = yaml.ScalarNode
			v.Tag = tag
			v.Value = value
			v.Style = 0
			return
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value},
	)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// guardWritePath enforces the cail-rules write boundary: only sources.yaml and
// files under reports/ or catalogue/ may be written (SPEC §2.1). Shared with
// vcs at milestone 9.
func guardWritePath(path string) error {
	clean := filepath.ToSlash(filepath.Clean(path))
	segs := strings.Split(clean, "/")
	for _, s := range segs {
		if s == "knowledge_base" || s == ".." {
			return fmt.Errorf("manifest: refusing to write outside the boundary: %s", path)
		}
	}
	switch filepath.Base(clean) {
	case "DECISIONS.md", "release.yaml", "CLAUDE.md":
		return fmt.Errorf("manifest: refusing to write protected file: %s", path)
	}
	if filepath.Base(clean) == "sources.yaml" {
		return nil
	}
	for _, s := range segs {
		if s == "reports" || s == "catalogue" {
			return nil
		}
	}
	return fmt.Errorf("manifest: %s is outside the writable set (sources.yaml, reports/, catalogue/)", path)
}
