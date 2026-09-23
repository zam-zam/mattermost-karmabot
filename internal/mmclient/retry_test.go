package mmclient

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// quietLogger keeps retry warnings out of test output.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fastTransport returns a retry transport with millisecond-scale delays so
// tests exercise the full retry sequence without sleeping.
func fastTransport(next http.RoundTripper) *retryTransport {
	return &retryTransport{
		next:        next,
		log:         quietLogger(),
		maxAttempts: 3,
		baseDelay:   time.Millisecond,
		maxDelay:    2 * time.Millisecond,
	}
}

// fakeTransport runs a fixed script of outcomes, one per attempt: a nil
// response means "return this error", otherwise the response is returned.
type fakeTransport struct {
	resps []*http.Response
	errs  []error
	calls atomic.Int32
}

func (f *fakeTransport) RoundTrip(*http.Request) (*http.Response, error) {
	i := int(f.calls.Add(1)) - 1
	if i >= len(f.resps) {
		i = len(f.resps) - 1
	}
	return f.resps[i], f.errs[i]
}

func statusResponse(status int) *http.Response {
	rec := httptest.NewRecorder()
	rec.WriteHeader(status)
	return rec.Result()
}

func TestRetryTransportSucceedsWithoutRetry(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := fastTransport(http.DefaultTransport).RoundTrip(
		mustRequest(t, http.MethodGet, srv.URL, nil))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits = %d, want 1", got)
	}
}

func TestRetryTransportRetriesStatusThenSucceeds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := fastTransport(http.DefaultTransport).RoundTrip(
		mustRequest(t, http.MethodGet, srv.URL, nil))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("server hits = %d, want 3 (two failures, then success)", got)
	}
}

func TestRetryTransportReturnsLastStatusAfterExhausting(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	resp, err := fastTransport(http.DefaultTransport).RoundTrip(
		mustRequest(t, http.MethodGet, srv.URL, nil))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want the final %d", resp.StatusCode, http.StatusBadGateway)
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("server hits = %d, want 3 (max attempts)", got)
	}
}

func TestRetryTransportDoesNotRetryClientErrors(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	resp, err := fastTransport(http.DefaultTransport).RoundTrip(
		mustRequest(t, http.MethodGet, srv.URL, nil))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits = %d, want 1 (404 is not retryable)", got)
	}
}

func TestRetryTransportRetries500OnlyForIdempotentMethods(t *testing.T) {
	tests := []struct {
		name   string
		method string
		status int
		hits   int32
	}{
		// A 500 on GET is safe to replay: the read has no side effects.
		{name: "get 500 retries", method: http.MethodGet, status: http.StatusInternalServerError, hits: 3},
		// A 500 on POST may mean the post was created and the failure came
		// after; replaying would duplicate it.
		{name: "post 500 does not retry", method: http.MethodPost, status: http.StatusInternalServerError, hits: 1},
		{name: "post 503 retries", method: http.MethodPost, status: http.StatusServiceUnavailable, hits: 3},
		{name: "post 429 retries", method: http.MethodPost, status: http.StatusTooManyRequests, hits: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			resp, err := fastTransport(http.DefaultTransport).RoundTrip(
				mustRequest(t, tt.method, srv.URL, strings.NewReader("payload")))
			if err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			defer resp.Body.Close()
			if got := hits.Load(); got != tt.hits {
				t.Errorf("server hits = %d, want %d", got, tt.hits)
			}
		})
	}
}

func TestRetryTransportReplaysRequestBody(t *testing.T) {
	var bodies []string
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading request body: %v", err)
		}
		bodies = append(bodies, string(b))
		if hits.Add(1) < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	resp, err := fastTransport(http.DefaultTransport).RoundTrip(
		mustRequest(t, http.MethodPost, srv.URL, strings.NewReader(`{"message":"++"}`)))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if len(bodies) != 2 {
		t.Fatalf("attempts = %d, want 2", len(bodies))
	}
	if bodies[0] != bodies[1] || bodies[0] == "" {
		t.Errorf("replayed body %q differs from original %q or is empty", bodies[1], bodies[0])
	}
}

func TestRetryTransportRetriesNetworkErrors(t *testing.T) {
	fake := &fakeTransport{
		resps: []*http.Response{nil, nil, statusResponse(http.StatusOK)},
		errs:  []error{errFakeNetwork, errFakeNetwork, nil},
	}

	resp, err := fastTransport(fake).RoundTrip(mustRequest(t, http.MethodGet, "http://karmabot.test", nil))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := fake.calls.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3 (fail, fail, succeed)", got)
	}
}

func TestRetryTransportGivesUpAfterNetworkErrors(t *testing.T) {
	fake := &fakeTransport{
		resps: []*http.Response{nil},
		errs:  []error{errFakeNetwork},
	}

	resp, err := fastTransport(fake).RoundTrip(mustRequest(t, http.MethodGet, "http://karmabot.test", nil))
	if err == nil {
		resp.Body.Close()
		t.Fatal("RoundTrip succeeded, want the transport error")
	}
	if got := fake.calls.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3 (max attempts)", got)
	}
}

func TestRetryTransportStopsWhenContextCancelled(t *testing.T) {
	// A fake transport is required: the real one fails before sending when
	// the context is already dead, which would not exercise our logic.
	fake := &fakeTransport{
		resps: []*http.Response{statusResponse(http.StatusServiceUnavailable)},
		errs:  []error{nil},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := mustRequest(t, http.MethodGet, "http://karmabot.test", nil).WithContext(ctx)

	// The 503 would normally be retried, but the dead context stops the
	// loop after the first attempt.
	resp, err := fastTransport(fake).RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	resp.Body.Close()
	if got := fake.calls.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (cancelled context stops retries)", got)
	}
}

func TestRetryTransportHonorsRetryAfterCappedByMaxDelay(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) < 2 {
			// 60s as asked, but maxDelay is 2ms in fastTransport: the wait
			// must be capped so the test does not actually sleep a minute.
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	start := time.Now()
	resp, err := fastTransport(http.DefaultTransport).RoundTrip(
		mustRequest(t, http.MethodGet, srv.URL, nil))
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Retry-After wait took %v, want it capped at maxDelay", elapsed)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("server hits = %d, want 2", got)
	}
}

func TestRetryAfterParsesHeader(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		maxWait time.Duration
		want    time.Duration
	}{
		{name: "seconds", header: "30", maxWait: time.Minute, want: 30 * time.Second},
		{name: "capped at max", header: "120", maxWait: 5 * time.Second, want: 5 * time.Second},
		{name: "absent", header: "", maxWait: time.Minute, want: 0},
		{name: "not a number", header: "soon", maxWait: time.Minute, want: 0},
		{name: "zero", header: "0", maxWait: time.Minute, want: 0},
		{name: "negative", header: "-5", maxWait: time.Minute, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := statusResponse(http.StatusTooManyRequests)
			if tt.header != "" {
				resp.Header.Set("Retry-After", tt.header)
			}
			if got := retryAfter(resp, tt.maxWait); got != tt.want {
				t.Errorf("retryAfter(Retry-After: %q, max %v) = %v, want %v",
					tt.header, tt.maxWait, got, tt.want)
			}
		})
	}
}

func TestJitteredStaysWithinHalfToFullDelay(t *testing.T) {
	d := 100 * time.Millisecond
	for range 100 {
		got := jittered(d)
		if got < d/2 || got >= d {
			t.Fatalf("jittered(%v) = %v, want in [%v, %v)", d, got, d/2, d)
		}
	}
	if got := jittered(0); got != 0 {
		t.Errorf("jittered(0) = %v, want 0", got)
	}
}

// errFakeNetwork stands in for transport-level errors like connection
// resets.
var errFakeNetwork = errors.New("fake network error")

func mustRequest(t *testing.T, method, url string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	return req
}
