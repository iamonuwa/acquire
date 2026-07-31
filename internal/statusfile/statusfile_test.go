package statusfile

import (
	"testing"
	"time"
)

var now = time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)

func TestBuild_FreshChange(t *testing.T) {
	st := Build(now, []Input{{ID: "nl", Jurisdiction: "NL", Changed: true, NormalizedHash: "abc", StalenessAlarmDays: 60}}, nil)
	s := st.Sources[0]
	if s.LastChange == nil || *s.LastChange != now.Format(time.RFC3339) {
		t.Errorf("last_change = %v, want now", s.LastChange)
	}
	if s.ConsecutiveFailures != 0 || s.Staleness != "ok" || s.GatePartial {
		t.Errorf("fresh NL change wrong: %+v", s)
	}
	if s.NormalizedHash == nil || *s.NormalizedHash != "abc" {
		t.Errorf("normalized_hash = %v", s.NormalizedHash)
	}
}

func TestBuild_ConsecutiveFailuresAccumulate(t *testing.T) {
	prev := &Status{Sources: []Source{{ID: "nl", ConsecutiveFailures: 2}}}
	st := Build(now, []Input{{ID: "nl", Jurisdiction: "NL", Failed: true, StalenessAlarmDays: 60}}, prev)
	if got := st.Sources[0].ConsecutiveFailures; got != 3 {
		t.Errorf("consecutive_failures = %d, want 3", got)
	}
}

func TestBuild_LastChangeCarriesOnNoChange(t *testing.T) {
	old := "2026-06-01T09:00:00Z"
	prev := &Status{Sources: []Source{{ID: "nl", LastChange: &old}}}
	st := Build(now, []Input{{ID: "nl", Jurisdiction: "NL", Changed: false, StalenessAlarmDays: 60}}, prev)
	if s := st.Sources[0]; s.LastChange == nil || *s.LastChange != old {
		t.Errorf("last_change = %v, want carried %q", s.LastChange, old)
	}
}

func TestBuild_Staleness(t *testing.T) {
	old := now.AddDate(0, 0, -100).Format(time.RFC3339)
	prev := &Status{Sources: []Source{{ID: "nl", LastChange: &old}}}
	st := Build(now, []Input{{ID: "nl", Jurisdiction: "NL", StalenessAlarmDays: 60}}, prev)
	if got := st.Sources[0].Staleness; got != "stale" {
		t.Errorf("staleness = %q, want stale (100d > 60d alarm)", got)
	}
}

func TestBuild_GatePartial(t *testing.T) {
	st := Build(now, []Input{
		{ID: "nl", Jurisdiction: "NL"},
		{ID: "dpd", Jurisdiction: "CA"},
		{ID: "bc", Jurisdiction: "BC"},
	}, nil)
	want := map[string]bool{"nl": false, "dpd": false, "bc": true}
	for _, s := range st.Sources {
		if s.GatePartial != want[s.ID] {
			t.Errorf("%s gate_partial = %v, want %v", s.ID, s.GatePartial, want[s.ID])
		}
	}
}

func TestRoundTrip(t *testing.T) {
	st := Build(now, []Input{{ID: "nl", Jurisdiction: "NL", Changed: true}}, nil)
	b, err := Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 1 || got.Sources[0].ID != "nl" {
		t.Errorf("round-trip lost data: %+v", got)
	}
}
