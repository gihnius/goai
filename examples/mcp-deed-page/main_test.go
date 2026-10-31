//go:build ignore

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zendev-sh/goai/mcp"
)

// Run explicitly, like the example itself:
// go test -race ./examples/mcp-deed-page/main.go ./examples/mcp-deed-page/main_test.go
func TestRun(t *testing.T) {
	file := filepath.Join(t.TempDir(), "report.html")
	if err := os.WriteFile(file, []byte("<h1>report</h1>"), 0o600); err != nil {
		t.Fatal(err)
	}
	sample := map[string]any{"files": map[string]any{"index.html": samplePage}, "ttl_seconds": float64(3600)}
	for _, tc := range []struct {
		name        string
		opts        options
		tool        string
		args        map[string]any
		toolError   bool
		outputError bool
	}{
		{"publish sample", options{ttl: 3600}, "publish_site", sample, false, false},
		{"CLI", options{}, "publish_site", sample, false, false},
		{"publish file", options{file: file, slug: "goai-report"}, "publish_site", map[string]any{"files": map[string]any{"index.html": "<h1>report</h1>"}, "slug": "goai-report"}, false, false},
		{"delete", options{deleteSlug: "goai-report", token: "deed_tok_test"}, "delete_site", map[string]any{"slug": "goai-report", "token": "deed_tok_test"}, false, false},
		{"tool error", options{ttl: 3600}, "publish_site", sample, true, false},
		{"RPC error", options{ttl: 3600}, "publish_site", sample, false, false},
		{"output error", options{ttl: 3600}, "", nil, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var methods []string
			listCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/mcp" || r.Header.Get("User-Agent") != userAgent {
					t.Errorf("request URL/header = %s, %q", r.URL, r.Header.Get("User-Agent"))
				}
				if r.Header.Get("Authorization") != "" {
					t.Error("example sent an Authorization header")
				}
				if r.Method == http.MethodGet {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				var msg mcp.JSONRPCMessage
				if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
					t.Error(err)
					return
				}
				methods = append(methods, msg.Method)
				var result any
				switch msg.Method {
				case "initialize":
					result = map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "test", "version": "1"}}
				case "notifications/initialized":
					w.WriteHeader(http.StatusAccepted)
					return
				case "tools/list":
					listCalls++
					var params mcp.ListParams
					if err := json.Unmarshal(msg.Params, &params); err != nil {
						t.Error(err)
					}
					if listCalls == 1 {
						result = map[string]any{"tools": []any{map[string]any{"name": "publish_site", "inputSchema": map[string]any{"type": "object"}}}, "nextCursor": "page2"}
					} else {
						if params.Cursor != "page2" {
							t.Errorf("cursor = %q", params.Cursor)
						}
						result = map[string]any{"tools": []any{map[string]any{"name": "delete_site", "inputSchema": map[string]any{"type": "object"}}}}
					}
				case "tools/call":
					var params struct {
						Name      string         `json:"name"`
						Arguments map[string]any `json:"arguments"`
					}
					if err := json.Unmarshal(msg.Params, &params); err != nil {
						t.Error(err)
					}
					if params.Name != tc.tool || !reflect.DeepEqual(params.Arguments, tc.args) {
						t.Errorf("call = %+v", params)
					}
					result = map[string]any{"content": []any{map[string]any{"type": "text", "text": `{"ok":true,"url":"https://goai-report.deed.page/"}`}}, "isError": tc.toolError}
				default:
					t.Errorf("unexpected method %s", msg.Method)
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.name == "RPC error" && msg.Method == "tools/call" {
					if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "error": map[string]any{"code": -32603, "message": "fixture failure"}}); err != nil {
						t.Error(err)
					}
					return
				}
				if err := json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result}); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			var out bytes.Buffer
			var writer io.Writer = &out
			if tc.outputError {
				writer = failingWriter{}
			}
			var err error
			if tc.name == "CLI" {
				out = *bytes.NewBufferString(runCLI(t, server.URL))
			} else {
				err = run(context.Background(), server.URL+"/mcp", server.Client(), tc.opts, writer)
			}
			if tc.outputError {
				if !errors.Is(err, io.ErrClosedPipe) {
					t.Fatalf("expected output error, got %v", err)
				}
				if want := []string{"initialize", "notifications/initialized", "tools/list"}; !reflect.DeepEqual(methods, want) {
					t.Errorf("methods = %v", methods)
				}
				return
			}
			if tc.name == "RPC error" {
				if err == nil || !strings.Contains(err.Error(), "fixture failure") {
					t.Fatalf("expected RPC error, got %v", err)
				}
			} else if tc.toolError {
				if err == nil || !strings.Contains(err.Error(), tc.tool) {
					t.Fatalf("expected tool error, got %v", err)
				}
			} else if err != nil || !strings.Contains(out.String(), "https://goai-report.deed.page/") {
				t.Fatalf("run = %v, output = %q", err, out.String())
			}
			if want := []string{"initialize", "notifications/initialized", "tools/list", "tools/list", "tools/call"}; !reflect.DeepEqual(methods, want) {
				t.Errorf("methods = %v", methods)
			}
		})
	}
}

// Exercise the default flags and endpoint through the real CLI.
// Only the network destination is redirected to the HTTP fixture.
func runCLI(t *testing.T, endpoint string) string {
	oldArgs, oldFlags, oldOut, oldTransport := os.Args, flag.CommandLine, os.Stdout, http.DefaultTransport
	t.Cleanup(func() {
		os.Args, flag.CommandLine, os.Stdout, http.DefaultTransport = oldArgs, oldFlags, oldOut, oldTransport
	})
	t.Setenv("DEED_PAGE_TOKEN", "")
	target, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != deedPageURL {
			t.Errorf("CLI endpoint = %s", r.URL)
		}
		clone := r.Clone(r.Context())
		clone.URL.Scheme, clone.URL.Host = target.Scheme, target.Host
		return oldTransport.RoundTrip(clone)
	})
	file, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	os.Stdout = file
	os.Args = []string{"mcp-deed-page"}
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	main()
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestDeleteNeedsToken(t *testing.T) {
	err := run(context.Background(), "http://127.0.0.1:0/mcp", http.DefaultClient, options{deleteSlug: "x"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "DEED_PAGE_TOKEN") {
		t.Fatalf("expected token error, got %v", err)
	}
}

func TestRunCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := run(ctx, deedPageURL, http.DefaultClient, options{ttl: 60}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected cancellation error")
	}
}
