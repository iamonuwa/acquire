package verifydin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlab.com/cail-health/cail-acquire/internal/fetch"
	"gitlab.com/cail-health/cail-acquire/internal/verifydin"
)

func fakeAPI(t *testing.T, records []map[string]string) *httptest.Server {
	t.Helper()
	body, _ := json.Marshal(records)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
}

func rec(din, brand string) map[string]string {
	return map[string]string{"drug_identification_number": din, "brand_name": brand}
}

func TestVerify_InCatalogue(t *testing.T) {
	idx := map[string][]string{"00000001": {"metformin-hydrochloride"}}
	// API base is unreachable on purpose; a catalogue hit must not call it.
	res, err := verifydin.Verify(context.Background(), fetch.NewClient(), "http://127.0.0.1:0", "00000001", idx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != verifydin.InCatalogue || len(res.Slugs) != 1 {
		t.Errorf("got %+v, want in_catalogue", res)
	}
}

func TestVerify_ExtractLagged(t *testing.T) {
	srv := fakeAPI(t, []map[string]string{rec("00000009", "LAGGED BRAND")})
	defer srv.Close()
	res, err := verifydin.Verify(context.Background(), fetch.NewClient(), srv.URL, "00000009", map[string][]string{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != verifydin.ExtractLagged || res.Brand != "LAGGED BRAND" {
		t.Errorf("got %+v, want extract_lagged", res)
	}
}

func TestVerify_NotFound(t *testing.T) {
	srv := fakeAPI(t, nil)
	defer srv.Close()
	res, err := verifydin.Verify(context.Background(), fetch.NewClient(), srv.URL, "00000009", map[string][]string{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != verifydin.NotFound {
		t.Errorf("got %+v, want not_found", res)
	}
}

// If the API ever regresses to ignoring din= and returns everything, the
// client-side filter must still give the right answer.
func TestVerify_ClientSideFilter(t *testing.T) {
	srv := fakeAPI(t, []map[string]string{rec("111", "A"), rec("222", "B")})
	defer srv.Close()

	hit, err := verifydin.Verify(context.Background(), fetch.NewClient(), srv.URL, "111", map[string][]string{})
	if err != nil || hit.Status != verifydin.ExtractLagged || hit.Brand != "A" {
		t.Errorf("din 111: got %+v, %v", hit, err)
	}
	miss, err := verifydin.Verify(context.Background(), fetch.NewClient(), srv.URL, "999", map[string][]string{})
	if err != nil || miss.Status != verifydin.NotFound {
		t.Errorf("din 999: got %+v, %v", miss, err)
	}
}
