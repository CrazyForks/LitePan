package fnosproxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"litepan/internal/playback"
	"litepan/internal/strm"
)

// 部分客户端要求媒体流必填字段均为非 null 字符串。
func TestNormalizeEmbyMediaStreams(t *testing.T) {
	tests := []struct {
		name        string
		stream      map[string]any
		wantChanged bool
	}{
		{
			name:        "缺 Title 与 DisplayLanguage",
			stream:      map[string]any{"Type": "Audio", "Language": "chi", "DisplayTitle": "[Mandarin]"},
			wantChanged: true,
		},
		{
			name:        "字段显式为 null",
			stream:      map[string]any{"Type": "Video", "Language": nil, "DisplayLanguage": nil, "Title": nil, "DisplayTitle": nil},
			wantChanged: true,
		},
		{
			name:        "字段齐全无需修改",
			stream:      map[string]any{"Type": "Video", "Language": "", "DisplayLanguage": "", "Title": "", "DisplayTitle": "4K HDR"},
			wantChanged: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := map[string]any{"MediaStreams": []any{tc.stream}}
			changed := normalizeEmbyMediaStreams(ms)
			if changed != tc.wantChanged {
				t.Fatalf("changed = %v, want %v", changed, tc.wantChanged)
			}
			for _, field := range embyMediaStreamNonNullFields {
				v, ok := tc.stream[field]
				if !ok {
					t.Errorf("字段 %q 补齐后仍缺失", field)
					continue
				}
				if _, isStr := v.(string); !isStr {
					t.Errorf("字段 %q 补齐后应为 string，实际 %T", field, v)
				}
			}
		})
	}
}

// 验证补齐后的结果可按客户端的严格结构解析。
func TestNormalizeEmbyMediaStreams_JSONParsable(t *testing.T) {
	ms := map[string]any{
		"MediaStreams": []any{
			map[string]any{"Type": "Video", "Codec": "hevc"},
			map[string]any{"Type": "Audio", "Language": "chi", "DisplayTitle": "DTS"},
		},
	}
	normalizeEmbyMediaStreams(ms)

	raw, err := json.Marshal(ms)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded struct {
		MediaStreams []map[string]json.RawMessage `json:"MediaStreams"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for i, stream := range decoded.MediaStreams {
		for _, field := range embyMediaStreamNonNullFields {
			v, ok := stream[field]
			if !ok || string(v) == "null" {
				t.Errorf("stream[%d] 字段 %q 缺失或为 null: ok=%v val=%s", i, field, ok, v)
			}
		}
	}
}

func TestNormalizeEmbyMediaStreams_NoStreams(t *testing.T) {
	for _, ms := range []map[string]any{
		nil,
		{},
		{"MediaStreams": "not-an-array"},
		{"MediaStreams": []any{}},
	} {
		if normalizeEmbyMediaStreams(ms) {
			t.Errorf("空/非法 MediaStreams 不应报告修改: %v", ms)
		}
	}
}

func TestRequestUpstreamWithRetryPreservesBody(t *testing.T) {
	var calls atomic.Int32
	requestBody := []byte(`{"EnableTranscoding":false}`)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("读取请求体失败: %v", err)
		}
		if !bytes.Equal(body, requestBody) {
			t.Errorf("第 %d 次请求体 = %q，期望 %q", calls.Load()+1, body, requestBody)
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaSources":[]}`))
	}))
	defer upstream.Close()

	service := New(Options{})
	request := httptest.NewRequest(http.MethodPost, "/Items/item-1/PlaybackInfo", bytes.NewReader(requestBody))
	response, body, err := service.requestUpstreamWithRetry(
		request,
		Config{FnosURL: upstream.URL},
		"emby/Items/item-1/PlaybackInfo",
		true,
	)
	if err != nil {
		t.Fatalf("基础重试失败: %v", err)
	}
	defer response.Body.Close()
	if calls.Load() != 2 {
		t.Fatalf("上游调用次数 = %d，期望 2", calls.Load())
	}
	if response.StatusCode != http.StatusOK || string(body) != `{"MediaSources":[]}` {
		t.Fatalf("最终响应 = (%d, %q)", response.StatusCode, body)
	}
}

func TestProxyRequestRewritesLocation(t *testing.T) {
	var upstreamURL string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/login" || r.URL.RawQuery != "next=%2Fhome" {
			t.Errorf("上游请求地址 = %q?%s", r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("X-Forwarded-Host") == "" || r.Header.Get("X-Forwarded-Proto") != "http" {
			t.Errorf("缺少标准转发头: host=%q proto=%q", r.Header.Get("X-Forwarded-Host"), r.Header.Get("X-Forwarded-Proto"))
		}
		if got := r.Header.Get("Authorization"); got != `MediaBrowser Token="test-token"` {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Location", upstreamURL+"/home")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()
	upstreamURL = upstream.URL

	service := New(Options{})
	var cfg Config
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service.proxyRequest(w, r, cfg, strings.TrimPrefix(r.URL.Path, "/"))
	}))
	defer proxyServer.Close()
	proxyURL, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatalf("解析反代地址失败: %v", err)
	}
	cfg = Config{FnosURL: upstream.URL, Port: proxyURL.Port()}

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest(http.MethodGet, proxyServer.URL+"/login?next=%2Fhome", nil)
	if err != nil {
		t.Fatalf("创建反代请求失败: %v", err)
	}
	req.Header.Set("Authorization", `MediaBrowser Token="test-token"`)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求反代失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("状态码 = %d，期望 %d", resp.StatusCode, http.StatusFound)
	}
	if got, want := resp.Header.Get("Location"), proxyServer.URL+"/home"; got != want {
		t.Fatalf("Location = %q，期望 %q", got, want)
	}
}

func TestProxyRequestSupportsProtocolUpgrade(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Connection"), "upgrade") ||
			!strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "缺少升级请求头", http.StatusUpgradeRequired)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "不支持连接接管", http.StatusInternalServerError)
			return
		}
		conn, rw, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		if err := rw.Flush(); err != nil {
			return
		}
		message := make([]byte, 4)
		if _, err := io.ReadFull(rw, message); err != nil {
			return
		}
		if string(message) != "ping" {
			return
		}
		_, _ = rw.WriteString("pong")
		_ = rw.Flush()
	}))
	defer upstream.Close()

	service := New(Options{})
	cfg := Config{FnosURL: upstream.URL}
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		service.proxyRequest(w, r, cfg, strings.TrimPrefix(r.URL.Path, "/"))
	}))
	defer proxyServer.Close()

	proxyURL, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatalf("解析反代地址失败: %v", err)
	}
	conn, err := net.DialTimeout("tcp", proxyURL.Host, 3*time.Second)
	if err != nil {
		t.Fatalf("连接反代失败: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = io.WriteString(conn,
		"GET /socket?api_key=test HTTP/1.1\r\nHost: "+proxyURL.Host+
			"\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n",
	)
	if err != nil {
		t.Fatalf("发送升级请求失败: %v", err)
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		t.Fatalf("读取升级响应失败: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("状态码 = %d，期望 %d", resp.StatusCode, http.StatusSwitchingProtocols)
	}
	if _, err := io.WriteString(conn, "ping"); err != nil {
		t.Fatalf("发送升级后数据失败: %v", err)
	}
	reply := make([]byte, 4)
	if _, err := io.ReadFull(reader, reply); err != nil {
		t.Fatalf("读取升级后数据失败: %v", err)
	}
	if string(reply) != "pong" {
		t.Fatalf("升级后响应 = %q，期望 pong", reply)
	}
}

func TestRedirectSTRMStreamNonInfuseUsesLitePanPlayback(t *testing.T) {
	fileID := "file-non-infuse"
	litepanURL := fmt.Sprintf("http://127.0.0.1:5211/api/strm/play/7/%s/t/token/n/demo.mkv", strm.EncodeFileKey(fileID))
	service := New(Options{})
	service.rememberSource("ms4", "item-4", "/movie/demo.strm", litepanURL)
	var gotReq playback.Request
	var gotIntent playback.Intent
	service.servePlayback = func(w http.ResponseWriter, r *http.Request, req playback.Request, intent playback.Intent) error {
		gotReq = req
		gotIntent = intent
		w.Header().Set("Location", "https://cdn.example/video.mkv")
		w.WriteHeader(http.StatusFound)
		return nil
	}

	req := httptest.NewRequest(http.MethodGet, "/Videos/item-4/stream?MediaSourceId=ms4", nil)
	req.Header.Set("User-Agent", "SenPlayer/1.0")
	rec := httptest.NewRecorder()

	service.redirectSTRMStream(rec, req, Config{}, strings.TrimPrefix(req.URL.RequestURI(), "/"))

	resp := rec.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("状态码=%d，期望 302", resp.StatusCode)
	}
	if gotReq.AccountID != 7 || gotReq.FileID != fileID {
		t.Fatalf("播放请求解析错误：account=%d file=%q", gotReq.AccountID, gotReq.FileID)
	}
	if gotIntent.FileName != "demo.mkv" || gotIntent.ForceProxy || gotIntent.Inline {
		t.Fatalf("播放意图错误：%+v", gotIntent)
	}
	if got := resp.Header.Get("Location"); got != "https://cdn.example/video.mkv" {
		t.Fatalf("Location=%q", got)
	}
}

func TestRedirectSTRMStreamNonLitePanStillRedirectsOriginalURL(t *testing.T) {
	service := New(Options{})
	service.rememberSource("ms5", "item-5", "/movie/demo.strm", "https://upstream.example/raw.m3u8")
	service.servePlayback = func(w http.ResponseWriter, r *http.Request, req playback.Request, intent playback.Intent) error {
		t.Fatal("非 LitePan 地址不应进入 playback")
		return nil
	}

	req := httptest.NewRequest(http.MethodGet, "/Videos/item-5/stream?MediaSourceId=ms5", nil)
	req.Header.Set("User-Agent", "Vidhub/1.0")
	rec := httptest.NewRecorder()

	service.redirectSTRMStream(rec, req, Config{}, strings.TrimPrefix(req.URL.RequestURI(), "/"))

	resp := rec.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("状态码=%d，期望 302", resp.StatusCode)
	}
	if got, want := resp.Header.Get("Location"), "https://upstream.example/raw.m3u8"; got != want {
		t.Fatalf("Location=%q，期望 %q", got, want)
	}
}

func TestRedirectSTRMStreamBrokenLitePanURLNoLongerFallsBackTo302(t *testing.T) {
	service := New(Options{})
	service.rememberSource("ms6", "item-6", "/movie/demo.strm", "http://127.0.0.1:5211/api/strm/play/broken")
	service.servePlayback = func(w http.ResponseWriter, r *http.Request, req playback.Request, intent playback.Intent) error {
		t.Fatal("坏 LitePan URL 不应进入 playback")
		return nil
	}

	req := httptest.NewRequest(http.MethodGet, "/Videos/item-6/stream?MediaSourceId=ms6", nil)
	req.Header.Set("User-Agent", "Vidhub/1.0")
	rec := httptest.NewRecorder()

	service.redirectSTRMStream(rec, req, Config{}, strings.TrimPrefix(req.URL.RequestURI(), "/"))

	resp := rec.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("状态码=%d，期望 502", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "" {
		t.Fatalf("Location=%q，期望为空", got)
	}
}

func TestRedirectSTRMStreamWrappedLitePanURLStillUsesPlayback(t *testing.T) {
	fileID := "wrapped-file"
	litepanURL := fmt.Sprintf("`http://127.0.0.1:5211/api/strm/play/7/%s/t/token/n/demo.mkv`", strm.EncodeFileKey(fileID))
	service := New(Options{})
	service.rememberSource("ms7", "item-7", "/movie/demo.strm", litepanURL)
	var gotReq playback.Request
	service.servePlayback = func(w http.ResponseWriter, r *http.Request, req playback.Request, intent playback.Intent) error {
		gotReq = req
		w.WriteHeader(http.StatusFound)
		return nil
	}

	req := httptest.NewRequest(http.MethodGet, "/Videos/item-7/stream?MediaSourceId=ms7", nil)
	rec := httptest.NewRecorder()
	service.redirectSTRMStream(rec, req, Config{}, strings.TrimPrefix(req.URL.RequestURI(), "/"))

	resp := rec.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("状态码=%d，期望 302", resp.StatusCode)
	}
	if gotReq.AccountID != 7 || gotReq.FileID != fileID {
		t.Fatalf("包裹反引号的 LitePan URL 解析错误：account=%d file=%q", gotReq.AccountID, gotReq.FileID)
	}
}

func TestRedirectSTRMStreamLitePanURLWithSpacesStillUsesPlayback(t *testing.T) {
	fileID := "file-with-spaces"
	fileName := "10间敢死队 (2026) [2160p].mkv"
	litepanURL := fmt.Sprintf("http://127.0.0.1:5211/api/strm/play/7/%s/t/token/n/%s", strm.EncodeFileKey(fileID), url.PathEscape(fileName))
	service := New(Options{})
	service.rememberSource("ms8", "item-8", "/movie/demo.strm", litepanURL)
	var gotReq playback.Request
	var gotIntent playback.Intent
	service.servePlayback = func(w http.ResponseWriter, r *http.Request, req playback.Request, intent playback.Intent) error {
		gotReq = req
		gotIntent = intent
		w.WriteHeader(http.StatusFound)
		return nil
	}

	req := httptest.NewRequest(http.MethodGet, "/Videos/item-8/stream?MediaSourceId=ms8", nil)
	rec := httptest.NewRecorder()
	service.redirectSTRMStream(rec, req, Config{}, strings.TrimPrefix(req.URL.RequestURI(), "/"))

	resp := rec.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("状态码=%d，期望 302", resp.StatusCode)
	}
	if gotReq.AccountID != 7 || gotReq.FileID != fileID {
		t.Fatalf("带空格文件名的 LitePan URL 解析错误：account=%d file=%q", gotReq.AccountID, gotReq.FileID)
	}
	if gotIntent.FileName != fileName {
		t.Fatalf("文件名=%q，期望 %q", gotIntent.FileName, fileName)
	}
}

func TestParseLitePanSTRMURLFilenameRegressionCases(t *testing.T) {
	cases := []struct {
		name     string
		fileName string
	}{
		{name: "中文空格括号", fileName: "10间敢死队 (2026) [2160p].mkv"},
		{name: "英文加号百分号", fileName: "Movie.Name.2024.2160p.HDR10+ 100%.mkv"},
		{name: "波浪线与符号", fileName: "A&B ~ Director's Cut, Final!.mp4"},
		{name: "全角符号混排", fileName: "全角～波浪＋中文＆英文【特别版】.mkv"},
		{name: "井号分号等号", fileName: "Episode 01; part=2 #remux!.mkv"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fileID := fmt.Sprintf("file-regression-%d", i)
			playURL := fmt.Sprintf(
				"http://127.0.0.1:5211/api/strm/play/7/%s/t/token/n/%s",
				strm.EncodeFileKey(fileID),
				url.PathEscape(tc.fileName),
			)
			accountID, gotFileID, ok := parseLitePanSTRMURL(playURL)
			if !ok {
				t.Fatalf("parseLitePanSTRMURL 返回 false，url=%q", playURL)
			}
			if accountID != 7 || gotFileID != fileID {
				t.Fatalf("解析结果错误：account=%d file=%q", accountID, gotFileID)
			}
			if gotName := strmFileNameFromPlayURL(playURL); gotName != tc.fileName {
				t.Fatalf("文件名=%q，期望 %q", gotName, tc.fileName)
			}
			if gotPath := litepanPath(playURL); strings.Contains(gotPath, " ") {
				t.Fatalf("编码路径不应出现空格：%q", gotPath)
			}
		})
	}
}

func TestSourceCacheSlidingExpirationAndLazyCleanup(t *testing.T) {
	service := New(Options{})
	service.rememberSource("mediasource_ms-1", "item-1", "/video/a.strm", "http://litepan/a")

	beforeAccess := time.Now().Add(-23 * time.Hour)
	service.cacheMu.Lock()
	service.byItem["item-1"].LastUsed = beforeAccess
	service.cacheMu.Unlock()

	got := service.lookupCached("ms-1", "")
	if got == nil {
		t.Fatal("24 小时内访问不应过期")
	}
	if !got.LastUsed.After(beforeAccess) {
		t.Fatalf("访问后 LastUsed = %v，未完成滑动续期", got.LastUsed)
	}

	service.cacheMu.Lock()
	service.byItem["item-1"].LastUsed = time.Now().Add(-sourceCacheTTL - time.Minute)
	service.cacheMu.Unlock()

	if got := service.lookupCached("", "item-1"); got != nil {
		t.Fatalf("超过 24 小时的条目仍可读取: %+v", got)
	}
	service.cacheMu.Lock()
	defer service.cacheMu.Unlock()
	if len(service.cacheEntries) != 0 || len(service.byMS) != 0 || len(service.byItem) != 0 {
		t.Fatalf(
			"过期条目未被懒清理: entries=%d byMS=%d byItem=%d",
			len(service.cacheEntries),
			len(service.byMS),
			len(service.byItem),
		)
	}
}

func TestSourceCacheMaxEntriesEvictsLeastRecentlyUsed(t *testing.T) {
	service := New(Options{})
	for i := 0; i < sourceCacheMaxEntries; i++ {
		service.rememberSource(
			fmt.Sprintf("mediasource_ms-%d", i),
			fmt.Sprintf("item-%d", i),
			fmt.Sprintf("/video/%d.strm", i),
			fmt.Sprintf("http://litepan/%d", i),
		)
	}

	service.cacheMu.Lock()
	service.byItem["item-0"].LastUsed = time.Now().Add(-time.Hour)
	if len(service.cacheEntries) != sourceCacheMaxEntries {
		t.Fatalf("真实条目数 = %d，期望 %d", len(service.cacheEntries), sourceCacheMaxEntries)
	}
	if len(service.byMS) != sourceCacheMaxEntries*2 {
		t.Fatalf("MediaSource 双索引数 = %d，期望 %d", len(service.byMS), sourceCacheMaxEntries*2)
	}
	service.cacheMu.Unlock()

	service.rememberSource("mediasource_ms-new", "item-new", "/video/new.strm", "http://litepan/new")

	service.cacheMu.Lock()
	entryCount := len(service.cacheEntries)
	service.cacheMu.Unlock()
	if entryCount != sourceCacheMaxEntries {
		t.Fatalf("淘汰后真实条目数 = %d，期望 %d", entryCount, sourceCacheMaxEntries)
	}
	if got := service.lookupCached("", "item-0"); got != nil {
		t.Fatalf("最久未使用条目未被淘汰: %+v", got)
	}
	if got := service.lookupCached("ms-new", ""); got == nil {
		t.Fatal("新写入条目被错误淘汰")
	}
}

func TestSourceCacheConfigChangeClearsEntries(t *testing.T) {
	service := New(Options{})
	initial := Config{
		FnosURL:  "http://fnos.local:5666/",
		PathMaps: "/vol1/media\n/vol2/media",
	}
	service.syncSourceCacheConfig(initial)
	service.rememberSource("mediasource_ms-1", "item-1", "/vol1/media/a.strm", "http://litepan/a")

	service.syncSourceCacheConfig(Config{
		FnosURL:  "http://fnos.local:5666",
		PathMaps: " /vol1/media\r\n/vol2/media/ ",
	})
	if got := service.lookupCached("", "item-1"); got == nil {
		t.Fatal("等价配置不应清空缓存")
	}

	service.syncSourceCacheConfig(Config{
		FnosURL:  "http://fnos.local:5666",
		PathMaps: "/vol3/media",
	})
	if got := service.lookupCached("", "item-1"); got != nil {
		t.Fatalf("STRM 路径映射变化后仍命中旧缓存: %+v", got)
	}

	service.rememberSource("mediasource_ms-2", "item-2", "/vol3/media/b.strm", "http://litepan/b")
	service.syncSourceCacheConfig(Config{
		FnosURL:  "http://fnos-new.local:5666",
		PathMaps: "/vol3/media",
	})
	if got := service.lookupCached("", "item-2"); got != nil {
		t.Fatalf("飞牛地址变化后仍命中旧缓存: %+v", got)
	}
}
