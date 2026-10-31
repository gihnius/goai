//go:build ignore

// Example: publish an HTML page to a public URL with the deed.page MCP server,
// then delete it. No account, API key or language model is required.
//
// Usage (from the repository root):
//
//	go run ./examples/mcp-deed-page/main.go
//	go run ./examples/mcp-deed-page/main.go -file report.html -ttl 0
//	DEED_PAGE_TOKEN=deed_tok_... go run ./examples/mcp-deed-page/main.go -delete <slug>
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/zendev-sh/goai/mcp"
)

const (
	deedPageURL = "https://deed.page/mcp"
	userAgent   = "goai-mcp-deed-page/1.0 (+https://github.com/zendev-sh/goai)"
	samplePage  = "<!doctype html><title>GoAI</title><h1>Published from GoAI over MCP</h1>"
)

type options struct {
	file, slug, deleteSlug, token string
	ttl                           int
}

func main() {
	var opts options
	flag.StringVar(&opts.file, "file", "", "HTML file to publish as index.html (default: a small sample page)")
	flag.StringVar(&opts.slug, "slug", "", "site name for https://{slug}.deed.page/ (default: random)")
	flag.IntVar(&opts.ttl, "ttl", 3600, "seconds until the page expires; 0 keeps it")
	flag.StringVar(&opts.deleteSlug, "delete", "", "delete this site instead of publishing (needs DEED_PAGE_TOKEN)")
	flag.Parse()
	opts.token = os.Getenv("DEED_PAGE_TOKEN")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := run(ctx, deedPageURL, &http.Client{Timeout: 60 * time.Second}, opts, os.Stdout); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(ctx context.Context, endpoint string, httpClient *http.Client, opts options, out io.Writer) error {
	name := "publish_site"
	var args map[string]any
	if opts.deleteSlug != "" {
		// The owner token from the first publish goes in the tool arguments.
		if opts.token == "" {
			return errors.New("-delete needs DEED_PAGE_TOKEN (the token returned by the first publish)")
		}
		name = "delete_site"
		args = map[string]any{"slug": opts.deleteSlug, "token": opts.token}
	} else {
		page := samplePage
		if opts.file != "" {
			data, err := os.ReadFile(opts.file)
			if err != nil {
				return err
			}
			page = string(data)
		}
		args = map[string]any{"files": map[string]any{"index.html": page}}
		if opts.slug != "" {
			args["slug"] = opts.slug
		}
		if opts.ttl > 0 {
			args["ttl_seconds"] = opts.ttl
		}
	}

	transport := mcp.NewHTTPTransport(endpoint,
		mcp.WithHTTPClient(httpClient),
		mcp.WithHTTPHeaders(map[string]string{"User-Agent": userAgent}),
	)
	client := mcp.NewClient("goai-mcp-deed-page", "1.0.0", mcp.WithTransport(transport))
	defer func() { _ = client.Close() }()
	if err := client.Connect(ctx); err != nil {
		return err
	}

	// Discover the tools before calling one.
	var cursor string
	for {
		tools, err := client.ListTools(ctx, &mcp.ListParams{Cursor: cursor})
		if err != nil {
			return err
		}
		for _, tool := range tools.Tools {
			if _, err := fmt.Fprintf(out, "Tool: %s\n", tool.Name); err != nil {
				return err
			}
		}
		cursor = tools.NextCursor
		if cursor == "" {
			break
		}
	}

	result, err := client.CallTool(ctx, name, args)
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("%s: %s", name, mcp.FormatContent(result.Content, false))
	}
	// The publish result holds the public url, the slug and, on the first
	// publish, the owner token. Keep that token private.
	_, err = fmt.Fprintln(out, mcp.FormatContent(result.Content, false))
	return err
}
