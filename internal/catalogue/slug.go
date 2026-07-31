// Package catalogue turns the DPD extract into the DIN-to-ingredient catalogue.
package catalogue

import (
	"regexp"
	"strings"
)

var slugSep = regexp.MustCompile(`[^a-z0-9]+`)

// Slug normalizes an ingredient name to a stable, deterministic slug: lowercase,
// with every run of non-alphanumeric characters collapsed to a single hyphen.
// It does not fold salt, ester, or hydrate forms into a base molecule, so
// "papaverine" and "papaverine hydrochloride" are distinct slugs.
func Slug(ingredient string) string {
	s := strings.ToLower(ingredient)
	s = slugSep.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
