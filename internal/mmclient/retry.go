package mmclient

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

const (
	// REST calls get up to 3 retries on top of the initial attempt, with
	// capped exponential backoff, mirroring the WebSocket reconnect
	// strategy in client.go.
	restMaxAttempts = 4
	restBaseDelay   = 500 * time.Millisecond
	restMaxDelay    = 5 * time.Second
)

// retryTransport wraps an http.RoundTripper and retries requests that hit a
// transient failure: network errors, rate limiting (429), and statuses that
// mean the request was almost certainly not processed (502, 503, and 500/504
// for idempotent methods only — replaying a POST that the server may have
// already applied would duplicate the message). It waits with jittered
// exponential backoff, honoring Retry-After when the server sends one.
type retryTransport struct {
	next        http.RoundTripper
	log         *slog.Logger
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
}

// newRetryTransport wraps next (http.DefaultTransport when nil) with the
// default retry settings.
func newRetryTransport(next http.RoundTripper, log *slog.Logger) *retryTransport {
	if next == nil {
		next = http.DefaultTransport
	}
	return &retryTransport{
		next:        next,
		log:         log,
		maxAttempts: restMaxAttempts,
		baseDelay:   restBaseDelay,
		maxDelay:    restMaxDelay,
	}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The request body is a one-shot stream; buffer it once so every
	// attempt replays the same bytes.
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			req.Body.Close()
			return nil, fmt.Errorf("buffering request body for retry: %w", err)
		}
		req.Body.Close()
	}

	delay := t.baseDelay
	for attempt := 1; ; attempt++ {
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
		}

		resp, err := t.next.RoundTrip(req)
		if attempt >= t.maxAttempts || !t.shouldRetry(req, resp, err) {
			return resp, err
		}

		wait := jittered(min(delay, t.maxDelay))
		if after := retryAfter(resp, t.maxDelay); after > wait {
			wait = after
		}
		t.logRetry(req, resp, err, attempt, wait)
		drain(resp)
		if !sleep(req.Context(), wait) {
			return nil, req.Context().Err()
		}
		delay *= 2
	}
}

func (t *retryTransport) shouldRetry(req *http.Request, resp *http.Response, err error) bool {
	if req.Context().Err() != nil {
		return false
	}
	if err != nil {
		return true
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests,
		http.StatusBadGateway,
		http.StatusServiceUnavailable:
		return true
	case http.StatusInternalServerError, http.StatusGatewayTimeout:
		return idempotent(req.Method)
	default:
		return false
	}
}

func (t *retryTransport) logRetry(req *http.Request, resp *http.Response, err error, attempt int, wait time.Duration) {
	if err != nil {
		t.log.Warn("mattermost rest call failed, retrying",
			"method", req.Method, "url", req.URL.Path,
			"err", err, "attempt", attempt, "retry_in", wait)
		return
	}
	t.log.Warn("mattermost rest call failed, retrying",
		"method", req.Method, "url", req.URL.Path,
		"status", resp.StatusCode, "attempt", attempt, "retry_in", wait)
}

// idempotent reports whether replaying the method cannot apply side effects
// twice. The zero value is treated as GET, matching http.Request semantics.
func idempotent(method string) bool {
	return method == "" || method == http.MethodGet || method == http.MethodHead
}

// retryAfter returns the wait requested by a Retry-After header in seconds
// form, capped at maxWait; 0 when absent or unparsable.
func retryAfter(resp *http.Response, maxWait time.Duration) time.Duration {
	if resp == nil {
		return 0
	}
	secs, err := strconv.Atoi(resp.Header.Get("Retry-After"))
	if err != nil || secs <= 0 {
		return 0
	}
	return min(time.Duration(secs)*time.Second, maxWait)
}

// jittered returns a random duration in [d/2, d).
func jittered(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return d/2 + time.Duration(rand.Int64N(int64(d)))/2
}

// drain discards the body of a response that is about to be retried past, so
// the underlying connection can be reused.
func drain(resp *http.Response) {
	if resp == nil {
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}
