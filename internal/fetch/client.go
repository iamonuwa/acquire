// Package fetch retrieves source payloads. Milestone 2 implements scrape_anchor
// hop 1 (URL resolution); the payload GET is milestone 3.
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

// Version is stamped at build time and appears in the User-Agent.
var Version = "0.0.0-dev"

const (
	userAgentProduct = "CAIL-acquire"

	// PLACEHOLDER — set to the real CAIL ops address before deploy.
	contactAddress = "ops@cail-health.example"

	connectTimeout   = 30 * time.Second
	totalTimeout     = 10 * time.Minute
	maxRedirects     = 5
	defaultRetries   = 3
	defaultRetryBase = 1 * time.Second
)

// errStopRedirect halts the redirect chain at the cap; it is not retried.
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

// WithRetryBase overrides the base backoff delay (default 1s).
func WithRetryBase(d time.Duration) Option { return func(c *Client) { c.retryBase = d } }

// WithUserAgent overrides the User-Agent.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// NewClient builds a Client with the standard timeouts, retry and redirect caps.
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

// HTTPStatusError is a non-2xx response. 5xx is retryable; 4xx is not.
type HTTPStatusError struct {
	URL        string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("fetch: %s returned HTTP %d", e.URL, e.StatusCode)
}

// Retryable reports whether the status warrants a retry (5xx only).
func (e *HTTPStatusError) Retryable() bool { return e.StatusCode >= 500 }

// Get issues a GET with retry/backoff and returns the body plus the final URL
// after redirects (the base for relative-anchor resolution).
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
	return b, resp.Request.URL, nil
}

func (c *Client) backoff(ctx context.Context, attempt int) error {
	d := c.retryBase << (attempt - 1)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// retryable reports whether an error warrants another attempt. Redirect-cap,
// context and 4xx errors are terminal; 5xx and network errors retry.
func retryable(err error) bool {
	if errors.Is(err, errStopRedirect) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var se *HTTPStatusError
	if errors.As(err, &se) {
		return se.Retryable()
	}
	return true
}
