package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zendev-sh/goai"
	"github.com/zendev-sh/goai/provider"
)

func TestVideo_ConstructorEnvironmentAndHelpers(t *testing.T) {
	t.Setenv("GOOGLE_GENERATIVE_AI_API_KEY", "env-key")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_GENERATIVE_AI_BASE_URL", "https://video.example.test")

	model := Video("veo-test").(*videoModel)
	if model.ModelID() != "veo-test" || model.opts.baseURL != "https://video.example.test" {
		t.Fatalf("model = %#v", model)
	}
	token, err := model.resolveToken(t.Context())
	if err != nil || token != "env-key" {
		t.Fatalf("token = %q, error = %v", token, err)
	}

	customClient := &http.Client{}
	model.opts.httpClient = customClient
	if model.httpClient() != customClient {
		t.Fatal("custom HTTP client not returned")
	}
	model.opts.httpClient = nil
	if model.httpClient() != http.DefaultClient {
		t.Fatal("default HTTP client not returned")
	}

	req := httptest.NewRequest(http.MethodGet, "https://video.example.test", nil)
	model.opts.headers = map[string]string{"X-Test": "value"}
	model.setHeaders(req, "key")
	if req.Header.Get("x-goog-api-key") != "key" || req.Header.Get("X-Test") != "value" {
		t.Fatalf("headers = %v", req.Header)
	}
}

func TestVideo_AuthenticationErrors(t *testing.T) {
	t.Setenv("GOOGLE_GENERATIVE_AI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_GENERATIVE_AI_BASE_URL", "")

	model := Video("veo-test").(*videoModel)
	if _, err := model.DoGenerate(t.Context(), provider.VideoParams{Prompt: "test"}); err == nil || !strings.Contains(err.Error(), "no API key") {
		t.Fatalf("error = %v", err)
	}

	model.opts.tokenSource = failingTokenSource{}
	if _, err := model.DoGenerate(t.Context(), provider.VideoParams{Prompt: "test"}); err == nil || !strings.Contains(err.Error(), "token fetch failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestVideo_RequestOptionsAndInlineResult(t *testing.T) {
	videoData := base64.StdEncoding.EncodeToString([]byte("inline-video"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "yes" {
			t.Errorf("custom header = %q", r.Header.Get("X-Custom"))
		}
		var body struct {
			Instances  []map[string]any `json:"instances"`
			Parameters map[string]any   `json:"parameters"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		instance := body.Instances[0]
		if instance["image"] == nil || instance["lastFrame"] == nil || instance["referenceImages"] == nil {
			t.Fatalf("instance = %#v", instance)
		}
		if body.Parameters["resolution"] != "720p" || body.Parameters["negativePrompt"] != "rain" {
			t.Fatalf("parameters = %#v", body.Parameters)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"name":"operations/inline","done":true,"response":{"generateVideoResponse":{"generatedSamples":[{"video":{"videoBytes":%q}}]}}}`, videoData)
	}))
	defer server.Close()

	model := Video("veo-test",
		WithAPIKey("key"),
		WithBaseURL(server.URL),
		WithHeaders(map[string]string{"X-Custom": "yes"}),
		WithHTTPClient(server.Client()),
	)
	result, err := model.DoGenerate(t.Context(), provider.VideoParams{
		Prompt:      "test",
		N:           1,
		Resolution:  "720p",
		PollTimeout: time.Second,
		FrameImages: []provider.VideoFrame{
			{Type: provider.VideoFrameFirst, Image: provider.MediaData{Data: []byte("first"), MediaType: "image/png"}},
			{Type: provider.VideoFrameLast, Image: provider.MediaData{Data: []byte("last"), MediaType: "image/png"}},
		},
		InputReferences: []provider.MediaData{{Data: []byte("reference"), MediaType: "image/png"}},
		ProviderOptions: map[string]any{"google": map[string]any{"negativePrompt": "rain"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Videos) != 1 || string(result.Videos[0].Data) != "inline-video" || result.Videos[0].MediaType != "video/mp4" {
		t.Fatalf("videos = %#v", result.Videos)
	}
}

func TestVideo_InputValidation(t *testing.T) {
	seed := int64(1)
	tests := []struct {
		name   string
		params provider.VideoParams
		want   string
	}{
		{name: "image URL", params: provider.VideoParams{Image: &provider.MediaData{URL: "https://example.com/image.png"}}, want: "inline image data"},
		{name: "empty image", params: provider.VideoParams{Image: &provider.MediaData{}}, want: "empty image data"},
		{name: "frame URL", params: provider.VideoParams{FrameImages: []provider.VideoFrame{{Type: provider.VideoFrameFirst, Image: provider.MediaData{URL: "https://example.com/image.png"}}}}, want: "inline image data"},
		{name: "fps", params: provider.VideoParams{FPS: 24}, want: "does not support fps"},
		{name: "seed", params: provider.VideoParams{Seed: &seed}, want: "does not support seed"},
		{name: "video reference", params: provider.VideoParams{InputReferences: []provider.MediaData{{Data: []byte("video"), MediaType: "video/mp4"}}}, want: "only supports image references"},
		{name: "reference URL", params: provider.VideoParams{InputReferences: []provider.MediaData{{URL: "https://example.com/image.png", MediaType: "image/png"}}}, want: "inline image data"},
	}
	model := &videoModel{id: "veo-test", opts: options{tokenSource: provider.StaticToken("key"), baseURL: "http://127.0.0.1:1"}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := model.DoGenerate(t.Context(), tt.params)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestVideo_EmptyOperationAndNoVideos(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     string
	}{
		{name: "empty operation", response: `{}`, want: "empty operation name"},
		{name: "no videos", response: `{"name":"operations/empty","done":true}`, want: "returned no videos"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tt.response)
			}))
			defer server.Close()
			model := Video("veo-test", WithAPIKey("key"), WithBaseURL(server.URL))
			_, err := model.DoGenerate(t.Context(), provider.VideoParams{Prompt: "test", N: 1})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestGoogleVideoOperation_VideoFileVariants(t *testing.T) {
	tests := []string{
		`{"response":{"generateVideoResponse":{"generatedVideos":[{"video":{"uri":"one"}}]}}}`,
		`{"response":{"generatedSamples":[{"video":{"uri":"two"}}]}}`,
		`{"response":{"generatedVideos":[{"video":{"uri":"three"}}]}}`,
	}
	for _, raw := range tests {
		var operation googleVideoOperation
		if err := json.Unmarshal([]byte(raw), &operation); err != nil {
			t.Fatal(err)
		}
		if files := operation.videoFiles(); len(files) != 1 || files[0].URI == "" {
			t.Fatalf("files = %#v", files)
		}
	}
	if files := (googleVideoOperation{}).videoFiles(); files != nil {
		t.Fatalf("files = %#v, want nil", files)
	}
}

func TestVideo_StartRetryEdges(t *testing.T) {
	t.Run("negative retries and network error", func(t *testing.T) {
		model := &videoModel{id: "veo-test", opts: options{
			baseURL: "https://video.example.test",
			httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("network failed")
			})},
		}}
		_, err := model.startOperation(t.Context(), "key", "https://video.example.test/start", nil, -1)
		if err == nil || !strings.Contains(err.Error(), "network failed") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("retry exhausted", func(t *testing.T) {
		model := rateLimitedVideoModel()
		_, err := model.startOperation(t.Context(), "key", "https://video.example.test/start", nil, 1)
		if err == nil || !strings.Contains(err.Error(), "1 retries exhausted") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("cancel during retry wait", func(t *testing.T) {
		model := rateLimitedVideoModel()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := model.startOperation(ctx, "key", "https://video.example.test/start", nil, 1)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	})
}

func rateLimitedVideoModel() *videoModel {
	return &videoModel{id: "veo-test", opts: options{
		baseURL: "https://video.example.test",
		httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     http.Header{"retry-after-ms": []string{"1"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"limited"}}`)),
			}, nil
		})},
	}}
}

func TestVideoRetryHelpers(t *testing.T) {
	if got := videoRetryAfter(&goai.APIError{ResponseHeaders: map[string]string{"retry-after": "2"}}); got != 2*time.Second {
		t.Fatalf("retry-after = %v", got)
	}
	if got := videoRetryAfter(&goai.APIError{ResponseHeaders: map[string]string{"retry-after-ms": "bad", "retry-after": "bad"}}); got != 0 {
		t.Fatalf("invalid retry-after = %v", got)
	}
	if got := videoExponentialDelay(99); got != 60*time.Second {
		t.Fatalf("capped delay = %v", got)
	}
	if err := waitForVideoRetry(t.Context(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func TestVideo_RequestOperationErrors(t *testing.T) {
	baseURL := "https://video.example.test"
	tests := []struct {
		name       string
		requestURL string
		transport  roundTripFunc
		want       string
	}{
		{name: "cross origin", requestURL: "https://other.example.test/start", want: "configured API origin"},
		{name: "transport", requestURL: baseURL + "/start", transport: func(*http.Request) (*http.Response, error) { return nil, errors.New("transport failed") }, want: "transport failed"},
		{name: "read", requestURL: baseURL + "/start", transport: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: &failReader{}}, nil
		}, want: "reading video response"},
		{name: "invalid JSON", requestURL: baseURL + "/start", transport: func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{"))}, nil
		}, want: "parsing video operation"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := &videoModel{opts: options{baseURL: baseURL}}
			if tt.transport != nil {
				model.opts.httpClient = &http.Client{Transport: tt.transport}
			}
			_, err := model.requestOperation(t.Context(), "key", http.MethodGet, tt.requestURL, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestVideo_PollAndOperationURLEdges(t *testing.T) {
	model := &videoModel{opts: options{baseURL: "https://video.example.test"}}
	done := googleVideoOperation{Name: "operations/done", Done: true}
	if result, err := model.pollOperation(t.Context(), "key", done, 0, 0); err != nil || result.Name != done.Name {
		t.Fatalf("result = %#v, error = %v", result, err)
	}
	if got, err := operationURL(model.opts.baseURL, "https://video.example.test/operations/one"); err != nil || got == "" {
		t.Fatalf("URL = %q, error = %v", got, err)
	}
	if got, err := operationURL(model.opts.baseURL+"/", "/operations/two"); err != nil || got != "https://video.example.test/v1beta/operations/two" {
		t.Fatalf("URL = %q, error = %v", got, err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := model.pollOperation(ctx, "key", googleVideoOperation{Name: "operations/wait"}, time.Second, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestVideo_DownloadInlineAndErrors(t *testing.T) {
	baseURL := "https://video.example.test"
	model := &videoModel{opts: options{baseURL: baseURL}}

	videos, err := model.downloadVideos(t.Context(), "key", []googleVideoFile{{
		BytesBase64Encoded: base64.StdEncoding.EncodeToString([]byte("video")),
	}})
	if err != nil || len(videos) != 1 || videos[0].MediaType != "video/mp4" {
		t.Fatalf("videos = %#v, error = %v", videos, err)
	}

	tests := []struct {
		name string
		file googleVideoFile
		want string
	}{
		{name: "invalid base64", file: googleVideoFile{BytesBase64Encoded: "%%%"}, want: "decoding video"},
		{name: "missing URI", file: googleVideoFile{}, want: "no data or URI"},
		{name: "cross origin", file: googleVideoFile{URI: "https://other.example.test/video.mp4"}, want: "configured API origin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := model.downloadVideos(t.Context(), "key", []googleVideoFile{tt.file})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestVideo_ReadAndRedirectHelpers(t *testing.T) {
	if _, err := readLimited(&failReader{}, 10); err == nil {
		t.Fatal("expected read error")
	}
	if _, err := readLimited(strings.NewReader("too long"), 2); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v", err)
	}
	if sameOrigin("%", "https://video.example.test") || sameOrigin("https://video.example.test", "%") {
		t.Fatal("invalid URL reported as same origin")
	}

	var previousCalled atomic.Bool
	model := &videoModel{opts: options{
		baseURL: "https://video.example.test",
		httpClient: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			previousCalled.Store(true)
			return errors.New("custom redirect policy")
		}},
	}}
	client := model.downloadHTTPClient()
	req := httptest.NewRequest(http.MethodGet, "https://video.example.test/next", nil)
	if err := client.CheckRedirect(req, nil); err == nil || !previousCalled.Load() {
		t.Fatalf("redirect error = %v, previous called = %v", err, previousCalled.Load())
	}

	model.opts.httpClient = &http.Client{}
	client = model.downloadHTTPClient()
	via := make([]*http.Request, 10)
	if err := client.CheckRedirect(req, via); err == nil || !strings.Contains(err.Error(), "10 redirects") {
		t.Fatalf("redirect error = %v", err)
	}
}
