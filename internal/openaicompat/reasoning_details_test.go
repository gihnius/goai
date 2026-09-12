package openaicompat

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/zendev-sh/goai/internal/sse"
	"github.com/zendev-sh/goai/provider"
)

func TestReasoningDetailsBoundaries(t *testing.T) {
	var a reasoningDetailsAccumulator
	input := []map[string]any{
		{"type": "reasoning.text", "text": "one", "id": "a", "index": 0},
		{"type": "reasoning.text", "text": "two", "id": "b", "index": 1},
		{"type": "reasoning.text", "signature": "sig", "id": "b", "index": 1},
		{"type": "reasoning.summary", "summary": "sum", "index": 1},
		{"type": "reasoning.encrypted", "data": "x", "index": 1},
		{"type": "reasoning.encrypted", "data": "y", "index": 1},
		{"type": "reasoning.future", "payload": map[string]any{"nested": true}},
	}
	before, _ := json.Marshal(input)
	if err := a.add(input); err != nil {
		t.Fatal(err)
	}
	if len(a.details) != 6 || a.details[1]["text"] != "two" || a.details[1]["signature"] != "sig" {
		t.Fatalf("wrong assembly: %#v", a.details)
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("mutated input")
	}
	if err := a.add([]map[string]any{{"type": "reasoning.text", "id": map[string]any{"bad": true}}, {"type": "reasoning.text", "id": map[string]any{"different": true}}}); err != nil {
		t.Fatal(err)
	}
}

func TestReasoningDetailsReplayOptIn(t *testing.T) {
	details := []map[string]any{{"type": "reasoning.text", "text": "think", "signature": "sig"}}
	messages := []provider.Message{{Role: provider.RoleAssistant, Content: []provider.Part{{Type: provider.PartReasoning, Text: "think", ProviderOptions: map[string]any{"openrouter": map[string]any{"reasoning_details": details}}}}}}
	for _, cfg := range []MessagesConfig{{}, {IncludeReasoningContent: true}} {
		body := ConvertMessagesWithConfig(messages, "", cfg)[0]
		if _, ok := body["reasoning_details"]; ok {
			t.Fatal("details leaked to non-opt-in provider")
		}
		if cfg.IncludeReasoningContent && body["reasoning_content"] != "think" {
			t.Fatal("existing reasoning_content behavior changed")
		}
	}
	// Message metadata takes precedence over part metadata and is not duplicated.
	messages[0].ProviderOptions = map[string]any{"openrouter": map[string]any{"reasoning_details": details}}
	body := ConvertMessagesWithConfig(messages, "", MessagesConfig{IncludeReasoningDetails: true})[0]
	if !reflect.DeepEqual(body["reasoning_details"], []any{details[0]}) {
		t.Fatalf("duplicated details: %#v", body)
	}
}

func TestReasoningDetailsStreamLimit(t *testing.T) {
	old := maxReasoningDetailsBytes
	maxReasoningDetailsBytes = 10
	t.Cleanup(func() { maxReasoningDetailsBytes = old })
	out := make(chan provider.StreamChunk, 5)
	go ParseStream(t.Context(), sse.NewScanner(strings.NewReader("data: {\"choices\":[{\"delta\":{\"reasoning_details\":[{\"type\":\"reasoning.encrypted\",\"data\":\"long\"}]}}]}\n\n")), out)
	var gotError bool
	for chunk := range out {
		if chunk.Type == provider.ChunkError {
			gotError = true
		}
		if chunk.Type == provider.ChunkFinish {
			t.Fatal("oversized state silently accepted")
		}
	}
	if !gotError {
		t.Fatal("missing size error")
	}
}
