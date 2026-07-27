package embyproxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"litepan/internal/settings"
	"litepan/internal/store"
)

func testEmbyProxyService(t *testing.T, embyURL string) *Service {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, store.Options{Memory: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	repos := store.New(db)
	settingsSvc, err := settings.New(ctx, repos.Configs)
	if err != nil {
		t.Fatal(err)
	}
	if err := settingsSvc.Update(ctx, map[string]string{
		settings.KeyEmbyEnabled:   "true",
		settings.KeyEmbyURL:       embyURL,
		settings.KeyEmbyAPIKey:    "test-key",
		settings.KeyEmbyProxyPort: "8097",
	}); err != nil {
		t.Fatal(err)
	}
	return New(Options{Settings: settingsSvc})
}

func TestDedicatedPortForwardsInfuseProbe(t *testing.T) {
	var gotPath, gotUA, gotAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUA = r.UserAgent()
		gotAuthorization = r.Header.Get("X-Emby-Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ServerName":"Emby"}`)
	}))
	defer upstream.Close()

	svc := testEmbyProxyService(t, upstream.URL)
	req := httptest.NewRequest(http.MethodGet, "http://litepan.test:8097/System/Info/Public", nil)
	req.Header.Set("User-Agent", "Infuse-Direct/8")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Emby-Authorization", `MediaBrowser Client="Infuse-Direct", Device="iPhone", DeviceId="test", Version="8"`)
	rec := httptest.NewRecorder()
	svc.handle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码=%d，响应=%s", rec.Code, rec.Body.String())
	}
	if gotPath != "/System/Info/Public" {
		t.Fatalf("上游路径=%q，期望 /System/Info/Public", gotPath)
	}
	if !strings.Contains(rec.Body.String(), `"ServerName":"Emby"`) {
		t.Fatalf("响应体=%q", rec.Body.String())
	}
	if gotUA != "Infuse-Direct/8" || !strings.Contains(gotAuthorization, `Client="Infuse-Direct"`) {
		t.Fatalf("Infuse 请求头未透传：UA=%q Authorization=%q", gotUA, gotAuthorization)
	}
}

func TestDedicatedPortPreservesInfuseAuthenticationBodyLength(t *testing.T) {
	var gotPath, gotBody, gotContentType string
	var gotContentLength int64
	var gotTransferEncoding []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		gotContentType = r.Header.Get("Content-Type")
		gotContentLength = r.ContentLength
		gotTransferEncoding = append([]string(nil), r.TransferEncoding...)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"AccessToken":"token","ServerId":"server"}`)
	}))
	defer upstream.Close()

	svc := testEmbyProxyService(t, upstream.URL)
	body := `{"Username":"user","Pw":"password"}`
	req := httptest.NewRequest(http.MethodPost, "http://litepan.test:8097/Users/AuthenticateByName", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Infuse-Direct/8")
	req.Header.Set("X-Emby-Authorization", `MediaBrowser Client="Infuse-Direct", Device="iPhone", DeviceId="test", Version="8"`)
	rec := httptest.NewRecorder()
	svc.handle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码=%d，响应=%s", rec.Code, rec.Body.String())
	}
	if gotPath != "/Users/AuthenticateByName" {
		t.Fatalf("上游路径=%q", gotPath)
	}
	if gotBody != body || gotContentType != "application/json" {
		t.Fatalf("认证请求体/类型未透传：body=%q type=%q", gotBody, gotContentType)
	}
	if gotContentLength != int64(len(body)) || len(gotTransferEncoding) != 0 {
		t.Fatalf("认证请求长度=%d，Transfer-Encoding=%v，期望固定 Content-Length=%d", gotContentLength, gotTransferEncoding, len(body))
	}
}
