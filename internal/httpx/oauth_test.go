package httpx

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// captureTransport 记录最后一个请求的 User-Agent，并返回 200 空响应。
type captureTransport struct {
	ua string
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.ua = req.Header.Get("User-Agent")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{}`)),
		Request:    req,
	}, nil
}

// TestPostOAuthProxyJSONCarriesUserAgent 保证发给 OAuth 代理的请求显式携带程序 UA，
// 便于服务端区分官方 litepan 与其它调用方。
func TestPostOAuthProxyJSONCarriesUserAgent(t *testing.T) {
	tr := &captureTransport{}
	client := NewClient(ClientOptions{})
	client.Transport = tr
	body := map[string]string{"driver_type": "onedrive", "refresh_token": "token"}
	if err := PostOAuthProxyJSON(context.Background(), client, "http://oauth.invalid/api/oauth/refresh", body, nil); err != nil {
		t.Fatal(err)
	}
	if tr.ua != DefaultUserAgent {
		t.Fatalf("User-Agent = %q，期望 %q", tr.ua, DefaultUserAgent)
	}
}
