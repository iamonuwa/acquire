// Package fetch retrieves source payloads. Milestone 2 implements the anchor
// resolution half of scrape_anchor (hop 1); the payload GET (hop 2) arrives at
// milestone 3.
//
// All HTTP goes through Client, whose settings are fixed by SPEC §6.1 so every
// request to a government host is uniform and polite.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Version is stamped at build time via -ldflags; it appears in the User-Agent.
var Version = "0.0.0-dev"

const (
	userAgentProduct = "CAIL-acquire"

	// contactAddress is the ops contact a publisher can reach (SPEC §6.1:
	// identify with a contact, do not spoof a browser).
	//
	// PLACEHOLDER — .example is a reserved domain. Set this to the real CAIL
	// ops address before deploy. Flagged rather than invented (CLAUDE rule 1).
	contactAddress = "ops@cail-health.example"

	connectTimeout   = 30 * time.Second
	totalTimeout     = 10 * time.Minute
	maxRedirects     = 5
	defaultRetries   = 3
	defaultRetryBase = 1 * time.Second
)

// errStopRedirect halts the client's redirect chain once the cap is reached.
// It is deterministic, so Get does not retry it.
var errStopRedirect = errors.New("fetch: redirect cap reached")

// Client is the shared HTTP client for all fetch strategies.
type Client struct {
	hc         *http.Client
	userAgent  string
	maxRetries int
	retryBase  time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithMaxRetries overrides the retry count (default 3).
func WithMaxRetries(n int) Option { return func(c *Client) { c.maxRetries = n } }

// WithRetryBase overrides the base backoff delay (default 1s). Tests set this
// small to keep retry cases fast.
func WithRetryBase(d time.Duration) Option { return func(c *Client) { c.retryBase = d } }

// WithUserAgent overrides the User-Agent (default identifies CAIL + contact).
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// NewClient builds a Client with the SPEC §6.1 defaults.
func NewClient(opts ...Option) *Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{Timeout: connectTimeout}).DialContext,
	}
	c := &Client{
		hc: &http.Client{
			Transport: transport,
			Timeout:   totalTimeout,
			CheckRedirect: func(_ *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return errStopRedirect
				}
				return nil
			},
		},
		userAgent:  fmt.Sprintf("%s/%s (+mailto:%s)", userAgentProduct, Version, contactAddress),
		maxRetries: defaultRetries,
		retryBase:  defaultRetryBase,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// HTTPStatusError is a non-2xx response. 5xx is retried; 4xx is not (SPEC §6.1).
type HTTPStatusError struct {
	URL        string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("fetch: %s returned HTTP %d", e.URL, e.StatusCode)
}

// Retryable reports whether the status warrants a retry (5xx only).
func (e *HTTPStatusError) Retryable() bool { return e.StatusCode >= 500 }

// Get issues a GET with retry/backoff and returns the fully-read body plus the
// final URL after redirects (needed as the base for relative-anchor resolution).
//
// Retries (up to maxRetries) fire on network errors and 5xx, with exponential
// backoff; 4xx and redirect-cap failures return immediately. Buffering the body
// lets a retry re-issue cleanly — index pages are small.
func (c *Client) Get(ctx context.Context, rawURL string) (body []byte, finalURL *url.URL, err error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			if werr := c.backoff(ctx, attempt); werr != nil {
				return nil, nil, werr
			}
		}
		body, finalURL, err = c.doOnce(ctx, rawURL)
		if err == nil {
			return body, finalURL, nil
		}
		lastErr = err
		if !retryable(err) {
			return nil, nil, err
		}
	}
	return nil, nil, fmt.Errorf("fetch: %s failed after %d attempts: %w", rawURL, c.maxRetries+1, lastErr)
}

func (c *Client) doOnce(ctx context.Context, rawURL string) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch: build request for %s: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, &HTTPStatusError{URL: rawURL, StatusCode: resp.StatusCode}
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch: read body from %s: %w", rawURL, err)
	}
	final := resp.Request.URL // reflects any redirects the client followed
	return b, final, nil
}

// backoff waits retryBase * 2^(attempt-1), honoring ctx cancellation.
func (c *Client) backoff(ctx context.Context, attempt int) error {
	d := c.retryBase << (attempt - 1)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// retryable reports whether an error from doOnce warrants another attempt.
// Redirect-cap and context errors are terminal; 4xx is terminal; a 5xx
// HTTPStatusError and any other (network) error are retryable.
func retryable(err error) bool {
	if errors.Is(err, errStopRedirect) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var se *HTTPStatusError
	if errors.As(err, &se) {
		return se.Retryable()
	}
	return true // network error
}
