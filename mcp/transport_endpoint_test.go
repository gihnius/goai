package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type endpointRoundTripper func(*http.Request) (*http.Response, error)

func (fn endpointRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestSSETransport_ResolveEndpointAndSend(t *testing.T) {
	for _, tt := range []struct {
		name, endpoint, wantURI                                  string
		absolute, invalidFirst, redirect, missingResponseRequest bool
	}{
		{name: "root relative", endpoint: "/messages?session_id=a%2Fb", wantURI: "/messages?session_id=a%2Fb"},
		{name: "path relative", endpoint: "messages?session_id=fixture", wantURI: "/nested/messages?session_id=fixture"},
		{name: "parent relative", endpoint: "../messages?session_id=fixture", wantURI: "/messages?session_id=fixture"},
		{name: "query relative", endpoint: "?session_id=fixture", wantURI: "/nested/sse?session_id=fixture"},
		{name: "redirected SSE", endpoint: "messages?session_id=fixture", wantURI: "/nested/messages?session_id=fixture", redirect: true},
		{name: "custom transport without response request", endpoint: "messages?session_id=fixture", wantURI: "/nested/messages?session_id=fixture", missingResponseRequest: true},
		{name: "absolute", endpoint: "/absolute?session_id=fixture", wantURI: "/absolute?session_id=fixture", absolute: true},
		{name: "invalid then valid", endpoint: "/messages?session_id=fixture", wantURI: "/messages?session_id=fixture", invalidFirst: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			type request struct {
				uri     string
				message JSONRPCMessage
			}
			posted := make(chan request, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					if tt.redirect && r.URL.Path == "/start" {
						http.Redirect(w, r, "/nested/sse?original=1", http.StatusFound)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					if tt.invalidFirst {
						_, _ = fmt.Fprint(w, "event: endpoint\ndata: /invalid%ZZ\n\n")
					}
					endpoint := tt.endpoint
					if tt.absolute {
						endpoint = "http://" + r.Host + endpoint
					}
					_, _ = fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpoint)
					w.(http.Flusher).Flush()
					<-r.Context().Done()
					return
				}
				var message JSONRPCMessage
				if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
					t.Errorf("decode POST: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				posted <- request{r.URL.RequestURI(), message}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"jsonrpc":"2.0","id":"1","result":{"ok":true}}`)
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			sseURL := server.URL + "/nested/sse?original=1"
			if tt.redirect {
				sseURL = server.URL + "/start"
			}
			transport := NewSSETransport(sseURL)
			if tt.missingResponseRequest {
				transport.HTTPClient = &http.Client{Transport: endpointRoundTripper(func(request *http.Request) (*http.Response, error) {
					response, err := http.DefaultTransport.RoundTrip(request)
					if response != nil {
						response.Request = nil
					}
					return response, err
				})}
			}
			failures := make(chan error, 4)
			transport.OnError(func(err error) {
				select {
				case failures <- err:
				default:
				}
			})
			responses := make(chan JSONRPCMessage, 1)
			transport.OnMessage(func(message JSONRPCMessage) { responses <- message })
			if err := transport.Start(ctx); err != nil {
				t.Fatalf("Start: %v", err)
			}
			defer func() { _ = transport.Close() }()
			if err := transport.Send(ctx, JSONRPCMessage{JSONRPC: "2.0", ID: "1", Method: "ping"}); err != nil {
				t.Fatalf("Send: %v", err)
			}
			select {
			case got := <-posted:
				if got.uri != tt.wantURI || got.message.Method != "ping" || got.message.ID != "1" {
					t.Fatalf("POST = %+v, want URI %q and ping id1", got, tt.wantURI)
				}
			case <-ctx.Done():
				t.Fatal("POST was not delivered")
			}
			select {
			case response := <-responses:
				if string(response.Result) != `{"ok":true}` {
					t.Fatalf("response = %s", response.Result)
				}
			case <-ctx.Done():
				t.Fatal("JSON response was not dispatched")
			}
			select {
			case err := <-failures:
				if !tt.invalidFirst || !strings.Contains(err.Error(), "resolve SSE endpoint") {
					t.Fatalf("unexpected transport error: %v", err)
				}
			default:
				if tt.invalidFirst {
					t.Fatal("invalid endpoint was not reported")
				}
			}
		})
	}
}
