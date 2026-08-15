package aiorganize

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"litepan/internal/mediaorganize/recognition"
	"litepan/internal/settings"
)

type configRepo struct {
	mu     sync.Mutex
	values map[string]string
}

func (r *configRepo) Get(_ context.Context, key string) (string, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	return value, ok, nil
}

func (r *configRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *configRepo) All(context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func newTestService(t *testing.T, transport roundTripFunc) *Service {
	t.Helper()
	settingsSvc, err := settings.New(context.Background(), &configRepo{values: map[string]string{
		settings.KeyAIOrganizeEnabled: "true",
		settings.KeyAIOrganizeBaseURL: "https://mock.invalid/v1",
		settings.KeyAIOrganizeAPIKey:  "test-key",
		settings.KeyAIOrganizeModel:   "test-model",
	}})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(settingsSvc)
	svc.http = &http.Client{Transport: transport}
	return svc
}

func TestConfigMasksStoredAPIKeyByLength(t *testing.T) {
	svc := newTestService(t, nil)
	cfg := svc.Config()
	if cfg.APIKey != "********" {
		t.Fatalf("API Key 脱敏长度 = %d，期望 8", len(cfg.APIKey))
	}
	if _, err := svc.Update(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if svc.runtimeConfig().APIKey != "test-key" {
		t.Fatal("保存脱敏值时不应覆盖原 API Key")
	}
}

func TestEnhanceValidatesAndCachesResult(t *testing.T) {
	calls := 0
	svc := newTestService(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("请求路径 = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatal("未携带 API Key")
		}
		return chatHTTPResponse(t, `{"items":[{"work_id":"work_1","recognized":true,"title":"千与千寻","year":2001,"media_type":"movie","files":[{"source_id":"source_1","kind":"movie"},{"source_id":"invented","episode":9}]}]}`), nil
	})
	req := recognition.BatchRequest{Works: []recognition.Work{{
		WorkID: "work_1",
		Files:  []recognition.File{{SourceID: "source_1", Name: "movie.mkv"}},
	}}}
	first, err := svc.Enhance(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].Title != "千与千寻" {
		t.Fatalf("识别结果异常: %+v", first)
	}
	if len(first.Items[0].Files) != 1 || first.Items[0].Files[0].SourceID != "source_1" {
		t.Fatalf("虚构文件未被过滤: %+v", first.Items[0].Files)
	}
	second, err := svc.Enhance(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if second.Cached != 1 || calls != 1 {
		t.Fatalf("缓存未命中: cached=%d calls=%d", second.Cached, calls)
	}
}

func TestEnhanceReportsBatchProgress(t *testing.T) {
	svc := newTestService(t, func(_ *http.Request) (*http.Response, error) {
		return chatHTTPResponse(t, `{"items":[{"work_id":"work_1","recognized":true,"title":"千与千寻","year":2001,"media_type":"movie"}]}`), nil
	})
	req := recognition.BatchRequest{Works: []recognition.Work{{
		WorkID: "work_1",
		Files:  []recognition.File{{SourceID: "source_1", Name: "movie.mkv"}},
	}}}
	states := make([]recognition.BatchProgress, 0)
	if _, err := svc.EnhanceWithProgress(context.Background(), req, func(state recognition.BatchProgress) {
		states = append(states, state)
	}); err != nil {
		t.Fatal(err)
	}
	last := states[len(states)-1]
	if last.Total != 1 || last.Completed != 1 || last.CurrentChunk != 1 || last.TotalChunks != 1 {
		t.Fatalf("进度不完整: %+v", states)
	}
	states = states[:0]
	if _, err := svc.EnhanceWithProgress(context.Background(), req, func(state recognition.BatchProgress) {
		states = append(states, state)
	}); err != nil {
		t.Fatal(err)
	}
	last = states[len(states)-1]
	if last.Completed != 1 || last.Cached != 1 || last.TotalChunks != 0 {
		t.Fatalf("缓存进度不正确: %+v", states)
	}
}

func TestEnhanceRepairsInvalidJSONOnce(t *testing.T) {
	calls := 0
	svc := newTestService(t, func(_ *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return chatHTTPResponse(t, "这不是 JSON"), nil
		}
		return chatHTTPResponse(t, `{"items":[{"work_id":"work_1","recognized":false}]}`), nil
	})
	result, err := svc.Enhance(context.Background(), recognition.BatchRequest{Works: []recognition.Work{{
		WorkID: "work_1",
		Files:  []recognition.File{{SourceID: "source_1", Name: "unknown.mkv"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(result.Items) != 1 || result.Items[0].Recognized {
		t.Fatalf("修复结果异常: calls=%d result=%+v", calls, result)
	}
}

func TestEnhanceKeepsSuccessfulChunksWhenAnotherChunkFails(t *testing.T) {
	calls := 0
	svc := newTestService(t, func(_ *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":"bad request"}`)),
			}, nil
		}
		return chatHTTPResponse(t, `{"items":[{"work_id":"work_21","recognized":true,"title":"Up","year":2009,"media_type":"movie"}]}`), nil
	})
	works := make([]recognition.Work, 0, 21)
	for i := 1; i <= 21; i++ {
		works = append(works, recognition.Work{
			WorkID: fmt.Sprintf("work_%d", i),
			Files:  []recognition.File{{SourceID: fmt.Sprintf("source_%d", i), Name: "movie.mkv"}},
		})
	}
	result, err := svc.Enhance(context.Background(), recognition.BatchRequest{Works: works})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || result.Failed != 20 || len(result.Items) != 1 || result.Items[0].WorkID != "work_21" {
		t.Fatalf("分片降级异常: calls=%d result=%+v", calls, result)
	}
}

func TestChatUsesAnthropicMessagesFormat(t *testing.T) {
	svc := newTestService(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("请求路径 = %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" || r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Fatalf("Anthropic 请求头不完整: %v", r.Header)
		}
		var payload struct {
			System    string        `json:"system"`
			MaxTokens int           `json:"max_tokens"`
			Messages  []chatMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.System == "" || payload.MaxTokens == 0 || len(payload.Messages) != 1 || payload.Messages[0].Role != "user" {
			t.Fatalf("Anthropic 请求体不正确: %+v", payload)
		}
		return anthropicHTTPResponse(t, `{"ok":true}`), nil
	})
	if err := svc.Test(context.Background(), Config{
		BaseURL: "https://api.anthropic.com",
		APIKey:  "test-key",
		Model:   "claude-sonnet-4-20250514",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestChatFallsBackAndRemembersAnthropicFormat(t *testing.T) {
	paths := make([]string, 0, 3)
	svc := newTestService(t, func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/v1/chat/completions" {
			return rawHTTPResponse(http.StatusNotFound, `{"error":"unknown endpoint"}`), nil
		}
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("意外请求路径 = %s", r.URL.Path)
		}
		return anthropicHTTPResponse(t, `{"ok":true}`), nil
	})
	cfg := Config{
		BaseURL: "https://mock.invalid/v1",
		APIKey:  "test-key",
		Model:   "test-model",
	}
	for range 2 {
		if err := svc.Test(context.Background(), cfg); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"/v1/chat/completions", "/v1/messages", "/v1/messages"}
	if fmt.Sprint(paths) != fmt.Sprint(want) {
		t.Fatalf("接口格式缓存异常: got=%v want=%v", paths, want)
	}
}

func TestConfigCannotEnableWhenIncomplete(t *testing.T) {
	settingsSvc, err := settings.New(context.Background(), &configRepo{values: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	svc := New(settingsSvc)
	if _, err := svc.Update(context.Background(), Config{Enabled: true}); err == nil {
		t.Fatal("配置不完整时不应允许启用")
	}
}

func chatHTTPResponse(t *testing.T, content string) *http.Response {
	t.Helper()
	var body strings.Builder
	if err := json.NewEncoder(&body).Encode(map[string]any{
		"choices": []any{map[string]any{"message": map[string]any{"content": content}}},
	}); err != nil {
		t.Fatal(err)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body.String())),
	}
}

func anthropicHTTPResponse(t *testing.T, content string) *http.Response {
	t.Helper()
	var body strings.Builder
	if err := json.NewEncoder(&body).Encode(map[string]any{
		"content": []any{map[string]any{"type": "text", "text": content}},
	}); err != nil {
		t.Fatal(err)
	}
	return rawHTTPResponse(http.StatusOK, body.String())
}

func rawHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
