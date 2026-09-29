package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/local/claude-relay/internal/config"
	"github.com/local/claude-relay/internal/cryptox"
	"github.com/local/claude-relay/internal/store"
)

func TestProxyMessagesAndStreamingUsage(t *testing.T) {
	var mu sync.Mutex
	seenKeys := make([]string, 0)
	upstreamHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seenKeys = append(seenKeys, r.Header.Get("x-api-key"))
		mu.Unlock()
		if r.URL.Path == "/v1/models" {
			writeJSON(w, http.StatusOK, map[string]any{"data": []map[string]any{{"id": "claude-sonnet-test"}, {"id": "claude-opus-test"}}})
			return
		}
		body, _ := io.ReadAll(r.Body)
		var input struct {
			Stream bool `json:"stream"`
		}
		_ = json.Unmarshal(body, &input)
		if input.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":12,\"output_tokens\":1}}}\n\n")
			_, _ = io.WriteString(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":7}}\n\n")
			_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "msg_test", "type": "message", "role": "assistant", "content": []any{},
			"model": "claude-sonnet-test", "stop_reason": "end_turn",
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	})

	dir := t.TempDir()
	vault, err := cryptox.NewVault(dir, "test-master-key")
	if err != nil {
		t.Fatal(err)
	}
	data, err := store.Open(dir+"/test.db", vault)
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	account, err := data.CreateAccount(context.Background(), store.AccountInput{
		Name: "upstream", BaseURL: "https://upstream.test", APIKey: "sk-ant-real-upstream", Enabled: true, Weight: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = account
	clientKey, err := data.CreateAPIKey(context.Background(), store.APIKeyInput{
		Name: "client", Enabled: true, AllowedModels: []string{"claude-sonnet-*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DataDir: dir, DatabasePath: dir + "/test.db", MaxRequestBodyBytes: 32 << 20}
	server, err := New(cfg, data, slog.New(slog.NewTextHandler(io.Discard, nil)), "test")
	if err != nil {
		t.Fatal(err)
	}
	server.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder := newResponseRecorder()
		upstreamHandler.ServeHTTP(recorder, request)
		return recorder.response(request), nil
	})}
	handler := server.Handler()

	call := func(stream bool) *memoryResponseWriter {
		body := `{"model":"claude-sonnet-test","max_tokens":10,"messages":[{"role":"user","content":"hi"}],"stream":` + map[bool]string{true: "true", false: "false"}[stream] + `}`
		request, _ := http.NewRequest(http.MethodPost, "http://relay.test/v1/messages", strings.NewReader(body))
		request.Header.Set("x-api-key", clientKey.Secret)
		request.Header.Set("anthropic-version", "2023-06-01")
		response := newResponseRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	nonStream := call(false)
	if nonStream.code != http.StatusOK || !strings.Contains(nonStream.body.String(), "msg_test") {
		t.Fatalf("non-stream response: status=%d body=%s", nonStream.code, nonStream.body.String())
	}
	stream := call(true)
	if stream.code != http.StatusOK || !strings.Contains(stream.body.String(), "message_stop") {
		t.Fatalf("stream response: status=%d body=%s", stream.code, stream.body.String())
	}
	logs, err := data.ListUsage(context.Background(), store.UsageFilter{Limit: 10})
	if err != nil || logs.Total != 2 {
		t.Fatalf("usage logs: %+v err=%v", logs, err)
	}
	var streamLog *store.UsageLog
	for i := range logs.Items {
		if logs.Items[i].Streamed {
			streamLog = &logs.Items[i]
		}
	}
	if streamLog == nil || streamLog.InputTokens != 12 || streamLog.OutputTokens != 7 {
		t.Fatalf("unexpected streaming usage: %+v", streamLog)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, key := range seenKeys {
		if key != "sk-ant-real-upstream" {
			t.Fatalf("client key leaked upstream: %q", key)
		}
	}

	restricted, _ := http.NewRequest(http.MethodPost, "http://relay.test/v1/messages", strings.NewReader(`{"model":"claude-opus-test","max_tokens":10,"messages":[]}`))
	restricted.Header.Set("x-api-key", clientKey.Secret)
	restrictedResponse := newResponseRecorder()
	handler.ServeHTTP(restrictedResponse, restricted)
	if restrictedResponse.code != http.StatusForbidden {
		t.Fatalf("restricted model status = %d, body=%s", restrictedResponse.code, restrictedResponse.body.String())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

type memoryResponseWriter struct {
	header http.Header
	body   strings.Builder
	code   int
}

func newResponseRecorder() *memoryResponseWriter {
	return &memoryResponseWriter{header: make(http.Header)}
}

func (r *memoryResponseWriter) Header() http.Header { return r.header }
func (r *memoryResponseWriter) WriteHeader(code int) {
	if r.code == 0 {
		r.code = code
	}
}
func (r *memoryResponseWriter) Write(data []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return r.body.Write(data)
}
func (r *memoryResponseWriter) Flush() {}
func (r *memoryResponseWriter) response(request *http.Request) *http.Response {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	return &http.Response{
		StatusCode: r.code, Status: http.StatusText(r.code), Header: r.header.Clone(),
		Body: io.NopCloser(strings.NewReader(r.body.String())), Request: request,
	}
}
