// Package normalize turns a raw payload into the text that gets hashed.
package normalize

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"

	"gitlab.com/cail-health/cail-acquire/internal/polltable"
)

const pdftotextBin = "pdftotext"

var pdftotextVersionRE = regexp.MustCompile(`version\s+(\S+)`)

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
func Apply(ctx context.Context, kind polltable.Normalize, raw []byte) ([]byte, error) {
	switch kind {
	case polltable.NormalizePDF:
		return PDF(ctx, raw)
	case polltable.NormalizeZipMembers:
		return nil, fmt.Errorf("normalize: zip_members not implemented yet")
	default:
		return nil, fmt.Errorf("normalize: unknown normalizer %q", kind)
	}
}

// Fingerprint returns the tool+version that produced the hash, e.g.
// "pdftotext-24.02.0", captured by running the tool.
func Fingerprint(ctx context.Context, kind polltable.Normalize) (string, error) {
	switch kind {
	case polltable.NormalizePDF:
		out, _ := exec.CommandContext(ctx, pdftotextBin, "-v").CombinedOutput()
		m := pdftotextVersionRE.FindSubmatch(out)
		if m == nil {
			return "", fmt.Errorf("normalize: cannot parse pdftotext version from %q", out)
		}
		return "pdftotext-" + string(m[1]), nil
	case polltable.NormalizeZipMembers:
		return "", fmt.Errorf("normalize: fingerprint for zip_members not implemented yet")
	default:
		return "", fmt.Errorf("normalize: unknown normalizer %q", kind)
	}
}
