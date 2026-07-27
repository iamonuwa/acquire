// Package normalize turns a raw payload into the text that gets hashed.
package normalize

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"gitlab.com/cail-health/cail-acquire/internal/config"
)

const pdftotextBin = "pdftotext"

// AssertAvailable checks that pdftotext is on PATH. Call at startup.
func AssertAvailable() error {
	if _, err := exec.LookPath(pdftotextBin); err != nil {
		return fmt.Errorf("normalize: %s not found on PATH: %w", pdftotextBin, err)
	}
	return nil
}

// PDF extracts text with `pdftotext -layout`; -layout is required to preserve
// the row-to-criterion association a reviewer reads.
func PDF(ctx context.Context, raw []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, pdftotextBin, "-layout", "-", "-")
	cmd.Stdin = bytes.NewReader(raw)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("normalize: pdftotext failed: %w: %s", err, stderr.String())
	}
	return out.Bytes(), nil
}

// Apply runs the normalizer named by kind.
func Apply(ctx context.Context, kind config.Normalize, raw []byte) ([]byte, error) {
	switch kind {
	case config.NormalizePDF:
		return PDF(ctx, raw)
	case config.NormalizeZipMembers:
		return nil, fmt.Errorf("normalize: zip_members not implemented yet")
	default:
		return nil, fmt.Errorf("normalize: unknown normalizer %q", kind)
	}
}
