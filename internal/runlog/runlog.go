// Package runlog records every fetch attempt, one JSON line per source per run.
// It is the only durable record of a run on which nothing changed.
package runlog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Record is one run's outcome for one source.
type Record struct {
	TS                    string  `json:"ts"`
	SourceID              string  `json:"source_id"`
	HTTPStatus            int     `json:"http_status"`
	FinalURL              string  `json:"final_url"`
	Bytes                 int     `json:"bytes"`
	LastModified          string  `json:"last_modified"`
	ETag                  string  `json:"etag"`
	RawHash               string  `json:"raw_hash"`
	NormalizedHash        string  `json:"normalized_hash"`
	NormalizerFingerprint string  `json:"normalizer_fingerprint"`
	Changed               bool    `json:"changed"`
	Error                 *string `json:"error"`
}

// Append writes rec as one JSON line to the append-only log at path.
func Append(path string, rec Record) error {
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("runlog: marshal: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("runlog: open %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("runlog: write %s: %w", path, err)
	}
	return nil
}

// SourceLines returns the log lines for one source, for mirroring to R2.
func SourceLines(path, sourceID string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("runlog: read %s: %w", path, err)
	}
	var out bytes.Buffer
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var r Record
		if json.Unmarshal(line, &r) == nil && r.SourceID == sourceID {
			out.Write(line)
			out.WriteByte('\n')
		}
	}
	return out.Bytes(), nil
}
