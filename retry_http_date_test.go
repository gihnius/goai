package goai_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider/compat"
)

func TestGenerateText_HTTPDateRetryAfter(t *testing.T) {
	deadline := time.Now().UTC().Add(30 * time.Second).Truncate(time.Second)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", deadline.Format(http.TimeFormat))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, `{"error":{"message":"rate limited"}}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var observed time.Duration
	var sawRetry bool
	before := time.Until(deadline)
	_, err := goai.GenerateText(ctx, compat.Chat("local-model", compat.WithBaseURL(server.URL)),
		goai.WithPrompt("hi"), goai.WithMaxRetries(1),
		goai.WithRetryObserver(func(_ int, _ error, delay time.Duration) { sawRetry = true; observed = delay; cancel() }),
	)
	after := time.Until(deadline)
	if !sawRetry || observed < after || observed > before {
		t.Fatalf("retry delay = %v (observed=%v), want between %v and %v", observed, sawRetry, after, before)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("requests = %d, want 1", calls.Load())
	}
}
