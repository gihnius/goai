package goai_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider/openai"
)

func TestStreamObjectEscapedUnicodeLiterals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		deltas []string
		want   []string
	}{
		{
			name:   "Windows path",
			deltas: []string{`{"value":"C:`, `\\u`, `sers"}`},
			want:   []string{`C:`, `C:\u`, `C:\users`},
		},
		{
			name:   "literal escape notation",
			deltas: []string{`{"value":"prefix`, `\\u123`, `suffix"}`},
			want:   []string{`prefix`, `prefix\u123`, `prefix\u123suffix`},
		},
		{
			name:   "real incomplete unicode escape",
			deltas: []string{`{"value":"prefix`, `\u12`, `34"}`},
			want:   []string{`prefix`, `prefix`, "prefix\u1234"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/chat/completions" {
					t.Errorf("request path = %q", r.URL.Path)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				for _, delta := range tc.deltas {
					data, err := json.Marshal(map[string]any{
						"choices": []any{map[string]any{
							"index": 0,
							"delta": map[string]any{"content": delta},
						}},
					})
					if err != nil {
						t.Error(err)
						return
					}
					if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
						t.Error(err)
						return
					}
				}
				if _, err := fmt.Fprint(w, "data: [DONE]\n\n"); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()

			type value struct {
				Value string `json:"value"`
			}
			model := openai.Chat("fixture", openai.WithAPIKey("local-fixture"), openai.WithBaseURL(server.URL))
			stream, err := goai.StreamObject[value](t.Context(), model,
				goai.WithPrompt("fixture"), goai.WithMaxRetries(0),
				goai.WithProviderOptions(map[string]any{"useResponsesAPI": false}))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for partial := range stream.PartialObjectStream() {
				got = append(got, partial.Value)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("partials = %q, want %q", got, tc.want)
			}
			result, err := stream.Result()
			if err != nil {
				t.Fatal(err)
			}
			if result.Object.Value != tc.want[len(tc.want)-1] {
				t.Errorf("final value = %q, want %q", result.Object.Value, tc.want[len(tc.want)-1])
			}
		})
	}
}
