---
title: Cheaper Inference Provider
description: "Access models from several labs through the Cheaper Inference OpenAI-compatible gateway in Go with GoAI, with chat, streaming and tool calling."
---

# Cheaper Inference

[Cheaper Inference](https://cheaperinference.com/) is an OpenAI-compatible LLM gateway. One API key gives access to models from several labs.

## Setup

```bash
go get github.com/zendev-sh/goai@latest
```

```go
import "github.com/zendev-sh/goai/provider/cheaperinference"
```

Set the `CHEAPER_INFERENCE_API_KEY` environment variable, or pass `WithAPIKey()` directly. Create a key at [cheaperinference.com](https://cheaperinference.com).

## Models

Model IDs are bare names:

- `gpt-5.4-mini`
- `gpt-5.4`
- `claude-sonnet-5`
- `deepseek-v4-flash`
- `glm-5.3`

`GET https://api.cheaperinference.com/public/models` lists the full catalog and needs no API key. See [cheaperinference.com/docs](https://cheaperinference.com/docs) for details.

## Tested Models

**Unit tested** (mock HTTP server): `gpt-5.4-mini`

## Usage

```go
model := cheaperinference.Chat("gpt-5.4-mini")

result, err := goai.GenerateText(ctx, model, goai.WithPrompt("Hello"))
if err != nil {
    log.Fatal(err)
}
fmt.Println(result.Text)
```

## Options

| Option | Type | Description |
|--------|------|-------------|
| `WithAPIKey(key)` | `string` | Set a static API key |
| `WithTokenSource(ts)` | `provider.TokenSource` | Set a dynamic token source |
| `WithBaseURL(url)` | `string` | Override the default `https://api.cheaperinference.com/v1` endpoint |
| `WithHeaders(h)` | `map[string]string` | Set additional HTTP headers |
| `WithHTTPClient(c)` | `*http.Client` | Set a custom `*http.Client` |

## Notes

- Declares text-only chat capability with tool calling. Image parts are sent in the standard `image_url` format for models that accept them.
- Streaming requests set `stream_options.include_usage` so the final chunk carries token usage.
- The package provides `Chat` only. Cheaper Inference also serves `/embeddings` (for example `text-embedding-3-small`), which `provider/compat` can reach with the same base URL.
- Environment variable `CHEAPER_INFERENCE_BASE_URL` can override the default endpoint.
