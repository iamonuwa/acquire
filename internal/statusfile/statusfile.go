// Package statusfile builds the run status file published to R2.
package statusfile

import (
	"encoding/json"
	"fmt"
	"time"
)

// Source is one source's status.
type Source struct {
	ID                  string  `json:"id"`
	LastChecked         string  `json:"last_checked"`
	LastChange          *string `json:"last_change"`
	LastPayloadURL      string  `json:"last_payload_url"`
	ConsecutiveFailures int     `json:"consecutive_failures"`
	NormalizedHash      *string `json:"normalized_hash"`
	Staleness           string  `json:"staleness"`
	GatePartial         bool    `json:"gate_partial"`
	OpenMR              *string `json:"open_mr"`
}

// Status is the full status file.
type Status struct {
	GeneratedAt string   `json:"generated_at"`
	Sources     []Source `json:"sources"`
}

// Input is one source's outcome for this run.
type Input struct {
	ID                 string
	Jurisdiction       string
	Failed             bool
	Changed            bool
	LastPayloadURL     string
	NormalizedHash     string
	StalenessAlarmDays int
}

// Build assembles the new status from this run's inputs and the previous status,
// carrying forward the stateful fields (consecutive failures, last change).
func Build(now time.Time, inputs []Input, prev *Status) *Status {
	prevByID := make(map[string]Source)
	if prev != nil {
		for _, s := range prev.Sources {
			prevByID[s.ID] = s
		}
	}
	nowStr := now.UTC().Format(time.RFC3339)
	st := &Status{GeneratedAt: nowStr}
	for _, in := range inputs {
		p := prevByID[in.ID]
		s := Source{
			ID:             in.ID,
			LastChecked:    nowStr,
			LastPayloadURL: in.LastPayloadURL,
			GatePartial:    gatePartial(in.Jurisdiction),
			OpenMR:         p.OpenMR,
			LastChange:     p.LastChange,
			NormalizedHash: p.NormalizedHash,
		}
		if in.Failed {
			s.ConsecutiveFailures = p.ConsecutiveFailures + 1
		}
		if in.Changed {
			s.LastChange = &nowStr
		}
		if in.NormalizedHash != "" && in.NormalizedHash != "UNVERIFIED" {
			h := in.NormalizedHash
			s.NormalizedHash = &h
		}
		s.Staleness = staleness(now, s.LastChange, in.StalenessAlarmDays)
		st.Sources = append(st.Sources, s)
	}
	return st
}

// Marshal renders the status as indented JSON.
func Marshal(s *Status) ([]byte, error) {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("statusfile: marshal: %w", err)
	}
	return append(b, '\n'), nil
}

// Parse reads a previously published status file.
func Parse(b []byte) (*Status, error) {
	var s Status
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("statusfile: parse: %w", err)
	}
	return &s, nil
}

func staleness(now time.Time, lastChange *string, alarmDays int) string {
	if lastChange == nil || alarmDays <= 0 {
		return "ok"
	}
	t, err := time.Parse(time.RFC3339, *lastChange)
	if err != nil {
		return "unknown"
	}
	if now.Sub(t) > time.Duration(alarmDays)*24*time.Hour {
		return "stale"
	}
	return "ok"
}

// gatePartial is true for a jurisdiction whose health-card format is not yet in
// the PHI gate. Only NL's MCP is implemented; CA is federal with no card.
func gatePartial(jurisdiction string) bool {
	switch jurisdiction {
	case "NL", "CA":
		return false
	default:
		return true
	}
}
