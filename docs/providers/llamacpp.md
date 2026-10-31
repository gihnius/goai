---
title: llama.cpp Provider
description: "Serve local open models with llama.cpp and GoAI. Use the llama.cpp server's OpenAI-compatible API for chat and embeddings with no API key required."
---

# llama.cpp

[llama.cpp](https://github.com/ggml-org/llama.cpp) server provider for local model inference. This GoAI provider wraps the generic [`compat`](compat.md) provider and requires no authentication by default.

## Setup

```bash
go get github.com/zendev-sh/goai@latest
```

```go
import "github.com/zendev-sh/goai/provider/llamacpp"
```

Start a llama.cpp server locally:

```bash
llama-server -m path/to/model.gguf
```

The server exposes an OpenAI-compatible API at `http://localhost:8080/v1` by default. No API key is required.

## Models

Any GGUF model served by a llama.cpp server. Common examples:

- `llama3`, `llama3.2:1b`, `llama3.2:3b`
- `mistral`, `mixtral`
- `qwen2.5`, `phi-3`
- embedding models for `Embedding`

## Usage

### Chat

```go
model := llamacpp.Chat("llama3")

result, err := goai.GenerateText(ctx, model, goai.WithPrompt("Hello"))
if err != nil {
    log.Fatal(err)
}
fmt.Println(result.Text)
```

### Embeddings

```go
embedModel := llamacpp.Embedding("nomic-embed-text")

result, err := goai.Embed(ctx, embedModel, "hello world")
```

### Remote llama.cpp Server

```go
model := llamacpp.Chat("llama3",
    llamacpp.WithBaseURL("http://192.168.1.100:8080/v1"),
)
```

## Options

| Option | Type | Description |
|--------|------|-------------|
| `WithAPIKey(key)` | `string` | Set a static API key (only if the server sits behind an auth-enforcing reverse proxy) |
| `WithTokenSource(ts)` | `provider.TokenSource` | Set a dynamic token source |
| `WithBaseURL(url)` | `string` | Override the default `http://localhost:8080/v1` endpoint |
| `WithHeaders(h)` | `map[string]string` | Set additional HTTP headers |
| `WithHTTPClient(c)` | `*http.Client` | Set a custom `*http.Client` |

## Notes

- This is a convenience wrapper around the [`compat`](compat.md) provider, using the llama.cpp server's OpenAI-compatible API.
- Default endpoint: `http://localhost:8080/v1`.
- llama.cpp is a local-first provider; the user explicitly controls the target URL, so SSRF-style host validation is intentionally not applied (same as `ollama`, `vllm`).