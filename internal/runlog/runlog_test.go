package runlog

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendAndSourceLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "runlog.jsonl")

	recs := []Record{
		{TS: "2026-07-28T09:00:00Z", SourceID: "nl", NormalizedHash: "sha256:aaa", Changed: true},
		{TS: "2026-07-29T09:00:00Z", SourceID: "dpd", NormalizedHash: "sha256:bbb", Changed: false},
		{TS: "2026-07-29T09:00:01Z", SourceID: "nl", NormalizedHash: "sha256:aaa", Changed: false},
	}
	for _, r := range recs {
		if err := Append(p, r); err != nil {
			t.Fatal(err)
		}
	}

	nl, err := SourceLines(p, "nl")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Count(strings.TrimSpace(string(nl)), "\n") + 1
	if lines != 2 {
		t.Errorf("nl lines = %d, want 2", lines)
	}
	if strings.Contains(string(nl), `"source_id":"dpd"`) {
		t.Error("nl slice leaked a dpd line")
	}
	if !strings.Contains(string(nl), `"changed":false`) || !strings.Contains(string(nl), `"changed":true`) {
		t.Error("nl slice missing a run")
	}
}
