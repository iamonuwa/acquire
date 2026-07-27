package fetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fastClient(opts ...Option) *Client {
	// Zero backoff base keeps retry tests instant.
	return NewClient(append([]Option{WithRetryBase(0)}, opts...)...)
}

func TestClient_RetriesOn5xx(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := fastClient(WithMaxRetries(3)).Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected failure after retries")
	}
	if got := hits.Load(); got != 4 { // 1 initial + 3 retries
		t.Errorf("server hits = %d, want 4", got)
	}
	var se *HTTPStatusError
	if !errors.As(err, &se) || se.StatusCode != 500 {
		t.Errorf("expected wrapped 500 HTTPStatusError, got %v", err)
	}
}

func TestClient_SucceedsAfterRetry(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	resp, err := fastClient(WithMaxRetries(3)).Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("expected success on 3rd attempt: %v", err)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("body = %q, want ok", resp.Body)
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("server hits = %d, want 3", got)
	}
}

func TestClient_NoRetryOn4xx(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := fastClient(WithMaxRetries(3)).Fetch(context.Background(), srv.URL)
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits = %d, want 1 (no retry on 4xx)", got)
	}
	var se *HTTPStatusError
	if !errors.As(err, &se) || se.StatusCode != 404 {
		t.Fatalf("expected 404 HTTPStatusError, got %v", err)
	}
	if se.Retryable() {
		t.Error("404 should not be retryable")
	}
}

func TestClient_RedirectCap(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		http.Redirect(w, r, fmt.Sprintf("/hop/%d", n), http.StatusFound)
	}))
	defer srv.Close()

	_, err := fastClient().Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected redirect-cap failure")
	}
	if !errors.Is(err, errStopRedirect) {
		t.Errorf("expected errStopRedirect, got %v", err)
	}
	// Cap is 5: at most 6 handler hits (initial + 5 followed), never unbounded.
	if got := hits.Load(); got > 6 {
		t.Errorf("server hits = %d, redirect chain not capped", got)
	}
}

func TestClient_UserAgentIdentifiesCAIL(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	if _, err := fastClient().Fetch(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ua, "CAIL-acquire/") {
		t.Errorf("User-Agent %q does not identify CAIL", ua)
	}
	if strings.Contains(strings.ToLower(ua), "mozilla") {
		t.Errorf("User-Agent %q spoofs a browser", ua)
	}
}

func TestClient_ContextCancelNotRetried(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	_, err := NewClient(WithRetryBase(time.Second)).Fetch(ctx, srv.URL)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
