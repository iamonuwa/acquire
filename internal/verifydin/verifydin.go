// Package verifydin resolves a single DIN against the catalogue and, as a
// fallback, the DPD JSON API. It is never a bulk source.
package verifydin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"gitlab.com/cail-health/cail-acquire/internal/fetch"
)

// DefaultAPIBase is the no-auth DPD JSON API.
const DefaultAPIBase = "https://health-products.canada.ca/api/drug"

// Status values for a lookup.
const (
	InCatalogue   = "in_catalogue"
	ExtractLagged = "extract_lagged"
	NotFound      = "not_found"
)

// Result is the outcome of verifying one DIN.
type Result struct {
	DIN    string
	Status string
	Slugs  []string
	Brand  string
}

type record struct {
	DIN   string `json:"drug_identification_number"`
	Brand string `json:"brand_name"`
}

// Verify checks din against the catalogue index first, then the DPD API. A DIN
// present in the API but absent from the catalogue is extract_lagged; it is
// never invented into a catalogue record.
func Verify(ctx context.Context, client *fetch.Client, apiBase, din string, dinIndex map[string][]string) (Result, error) {
	if slugs, ok := dinIndex[din]; ok {
		return Result{DIN: din, Status: InCatalogue, Slugs: slugs}, nil
	}
	recs, err := apiLookup(ctx, client, apiBase, din)
	if err != nil {
		return Result{}, err
	}
	if len(recs) > 0 {
		return Result{DIN: din, Status: ExtractLagged, Brand: recs[0].Brand}, nil
	}
	return Result{DIN: din, Status: NotFound}, nil
}

// apiLookup queries with ?din= and then filters client-side for an exact match,
// so the result is correct whether or not the API honors the filter.
func apiLookup(ctx context.Context, client *fetch.Client, apiBase, din string) ([]record, error) {
	u := fmt.Sprintf("%s/drugproduct/?lang=en&type=json&din=%s", apiBase, url.QueryEscape(din))
	resp, err := client.Fetch(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("verifydin: api lookup: %w", err)
	}
	var recs []record
	if err := json.Unmarshal(resp.Body, &recs); err != nil {
		return nil, fmt.Errorf("verifydin: parse api response: %w", err)
	}
	var out []record
	for _, r := range recs {
		if r.DIN == din {
			out = append(out, r)
		}
	}
	return out, nil
}
