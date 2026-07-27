// Package gate is the PHI hard stop that fires before any write.
package gate

// Result is the outcome of a PHI scan.
type Result struct {
	Hit    bool
	Reason string
}

// Check scans normalized text for PHI.
//
// PLACEHOLDER: always reports no hit. The real SIN and NL MCP detectors come
// later; the call site is wired now so the gate runs before any store write.
func Check(text []byte) Result {
	return Result{}
}
