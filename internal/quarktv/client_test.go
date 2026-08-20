package quarktv

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"litepan/internal/domain"
)

func TestPickStreamingCandidatePrefersDolbyOnlyWhenEnabled(t *testing.T) {
	infos := []streamingVideoInfo{
		{Resolution: "4k", Accessable: 1, Format: "mp4", URL: "https://example/4k"},
		{Resolution: "dolby_vision", Accessable: 1, Format: "matroska,webm", URL: "https://example/dv"},
	}

	gotOff, ok := pickStreamingCandidate("fid", infos, StreamingPreference{
		PreferredResolution: domain.QuarkTVResolutionAuto,
		AllowDolby:          false,
	}, nil)
	if !ok {
		t.Fatal("dolby 关闭时未选出候选")
	}
	if gotOff.Resolution != "4k" {
		t.Fatalf("dolby 关闭时应选 4k，实际为 %q", gotOff.Resolution)
	}

	gotOn, ok := pickStreamingCandidate("fid", infos, StreamingPreference{
		PreferredResolution: domain.QuarkTVResolutionAuto,
		AllowDolby:          true,
	}, nil)
	if !ok {
		t.Fatal("dolby 开启时未选出候选")
	}
	if gotOn.Resolution != "dolby_vision" {
		t.Fatalf("dolby 开启时应选 dolby_vision，实际为 %q", gotOn.Resolution)
	}
}

func TestPickStreamingCandidateRespectsResolutionCap(t *testing.T) {
	infos := []streamingVideoInfo{
		{Resolution: "4k", Accessable: 1, Format: "mp4", URL: "https://example/4k"},
		{Resolution: "super", Accessable: 1, Format: "mp4", URL: "https://example/super"},
		{Resolution: "high", Accessable: 1, Format: "mp4", URL: "https://example/high"},
	}

	got, ok := pickStreamingCandidate("fid", infos, StreamingPreference{
		PreferredResolution: domain.QuarkTVResolutionHigh,
		AllowDolby:          false,
	}, nil)
	if !ok {
		t.Fatal("受限清晰度下未选出候选")
	}
	if got.Resolution != "high" {
		t.Fatalf("清晰度上限为 high 时应选 high，实际为 %q", got.Resolution)
	}
}

func TestPickStreamingCandidateTreats2KAsSuperBucket(t *testing.T) {
	infos := []streamingVideoInfo{
		{Resolution: "4k", Accessable: 1, Format: "mp4", URL: "https://example/4k"},
		{Resolution: "2k", Accessable: 1, Format: "mp4", URL: "https://example/2k"},
		{Resolution: "super", Accessable: 1, Format: "mp4", URL: "https://example/super"},
		{Resolution: "high", Accessable: 1, Format: "mp4", URL: "https://example/high"},
	}

	got, ok := pickStreamingCandidate("fid", infos, StreamingPreference{
		PreferredResolution: domain.QuarkTVResolutionSuper,
		AllowDolby:          false,
	}, nil)
	if !ok {
		t.Fatal("超清档未选出候选")
	}
	if got.Resolution != "2k" {
		t.Fatalf("超清档应优先命中 2k，实际为 %q", got.Resolution)
	}
}

func TestPickStreamingCandidateFallsBackFrom4KTo2K(t *testing.T) {
	infos := []streamingVideoInfo{
		{Resolution: "2k", Accessable: 1, Format: "mp4", URL: "https://example/2k"},
		{Resolution: "super", Accessable: 1, Format: "mp4", URL: "https://example/super"},
	}

	got, ok := pickStreamingCandidate("fid", infos, StreamingPreference{
		PreferredResolution: domain.QuarkTVResolution4K,
		AllowDolby:          false,
	}, nil)
	if !ok {
		t.Fatal("4k 档未选出候选")
	}
	if got.Resolution != "2k" {
		t.Fatalf("4k 档缺失时应回落到 2k，实际为 %q", got.Resolution)
	}
}

func TestParseQuarkTVHTTPErrorMessageMapsDeviceLimit(t *testing.T) {
	body := []byte(`{"code":400,"message":"device limit exceeded"}`)
	got := parseQuarkTVHTTPErrorMessage(body)
	want := "设备数超限"
	if got != want {
		t.Fatalf("parseQuarkTVHTTPErrorMessage() = %q, want %q", got, want)
	}
}

func TestExchangeTokenReturnsBodyMessageOnHTTP400(t *testing.T) {
	client := NewClient("device-id", "", "", tokenExpiresAt(3600))
	client.http = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != codeAPI+"/token" {
				t.Fatalf("unexpected url: %s", req.URL.String())
			}
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"code":400,"message":"device limit exceeded"}`)),
				Request:    req,
			}, nil
		}),
	}

	_, err := client.exchangeToken(context.Background(), "device-id", "bind-code", false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := "DRIVER_ERROR: 设备数超限"
	if err.Error() != want {
		t.Fatalf("exchangeToken error = %q, want %q", err.Error(), want)
	}
}

func TestDoOnceReturnsBodyErrorMessageOnHTTP400(t *testing.T) {
	client := NewClient("device-id", "refresh-token", "access-token", tokenExpiresAt(3600))
	client.http = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/user" {
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"status":-1,"errno":32009,"error_info":"设备数超限"}`)),
				Request:    req,
			}, nil
		}),
	}

	_, err := client.doOnce(context.Background(), http.MethodGet, "/user", nil, nil, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := "DRIVER_ERROR: 设备数超限"
	if err.Error() != want {
		t.Fatalf("doOnce error = %q, want %q", err.Error(), want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
