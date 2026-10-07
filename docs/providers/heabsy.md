---
title: Heabsy Provider
description: "Use open models through Heabsy's OpenAI-compatible inference API in Go with GoAI. Chat, streaming and tool calling with one API key."
---

# Heabsy

[Heabsy](https://heabsy.com/platform) is an OpenAI-compatible inference API for open models. Models in the EEA tier run on dedicated GPUs in EEA data centres with zero data retention; models routed through third parties are labelled as such in the catalog.

## Setup

```bash
go get github.com/zendev-sh/goai@latest
```

```go
import "github.com/zendev-sh/goai/provider/heabsy"
```

Set the `HEABSY_API_KEY` environment variable, or pass `WithAPIKey()` directly. Get an API key in the [Heabsy console](https://platform.heabsy.com); accounts are opened on request at [heabsy.com/contacts](https://heabsy.com/contacts).

## Models

Model IDs are the names returned by `GET /v1/models`, for example:

- `qwen38` (Qwen3.8 27B: 262,144-token context, up to 32,768 output tokens, image input, tool calling, structured output)

See the [Heabsy model catalog](https://heabsy.com/models) for the full list and prices.

## Tested Models

**Unit tested** (mock HTTP server): `qwen38`

## Usage

```go
model := heabsy.Chat("qwen38")

result, err := goai.GenerateText(ctx, model, goai.WithPrompt("Hello"))
if err != nil {
    log.Fatal(err)
}
fmt.Println(result.Text)
```

### Reasoning on/off

`qwen38` switches reasoning on or off with `chat_template_kwargs.enable_thinking`. Pass it with `goai.WithProviderOptions`; unknown keys are sent in the request body as-is:

```go
result, err := goai.GenerateText(ctx, model,
    goai.WithPrompt("Hello"),
    goai.WithProviderOptions(map[string]any{
        "chat_template_kwargs": map[string]any{"enable_thinking": false},
    }),
)
```

## Options

| Option | Type | Description |
|--------|------|-------------|
| `WithAPIKey(key)` | `string` | Set a static API key |
| `WithTokenSource(ts)` | `provider.TokenSource` | Set a dynamic token source |
| `WithBaseURL(url)` | `string` | Override the default `https://api.heabsy.com/v1` endpoint |
| `WithHeaders(h)` | `map[string]string` | Set additional HTTP headers |
| `WithHTTPClient(c)` | `*http.Client` | Set a custom `*http.Client` |

## Notes

- Image input works with models that accept it, such as `qwen38`.
- Heabsy has no embeddings endpoint, so the package provides `Chat` only.
- Environment variable `HEABSY_BASE_URL` can override the default endpoint.
