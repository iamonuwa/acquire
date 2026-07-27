package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDispatch_UnknownAndReserved(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"no args", nil, exitConfig},
		{"unknown", []string{"frobnicate"}, exitConfig},
		{"reserved catalogue", []string{"catalogue"}, exitConfig},
		{"reserved status", []string{"status"}, exitConfig},
		{"reserved verify-din", []string{"verify-din"}, exitConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := dispatch(tc.args); got != tc.want {
				t.Errorf("dispatch(%v) = %d, want %d", tc.args, got, tc.want)
			}
		})
	}
}

func TestRunPoll_BadManifest(t *testing.T) {
	if got := runPoll([]string{"--manifest", filepath.Join(t.TempDir(), "nope.yaml")}); got != exitConfig {
		t.Errorf("runPoll with missing manifest = %d, want %d", got, exitConfig)
	}
}

const validManifest = `- id: nlpdp-sa-criteria
  url: https://www.gov.nl.ca/hcs/prescription/covered-specialauthdrugs/
  url_kind: index
  state: active
- id: hc-dpd-allfiles
  url: https://www.canada.ca/example.html
  url_kind: index
  state: active
`

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sources.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunPoll_SourceNoMatch(t *testing.T) {
	mp := writeTemp(t, validManifest)
	if got := runPoll([]string{"--manifest", mp, "--source", "does-not-exist"}); got != exitConfig {
		t.Errorf("runPoll with unmatched --source = %d, want %d", got, exitConfig)
	}
}

func TestRunPoll_BadConfigPath(t *testing.T) {
	if got := runPoll([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml")}); got != exitConfig {
		t.Errorf("runPoll with missing --config = %d, want %d", got, exitConfig)
	}
}
