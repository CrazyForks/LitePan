package strm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"litepan/internal/playback"
)

const (
	metadataHTTPAttempts  = 3
	metadataResolveRounds = 3
	metadataClientTimeout = 5 * time.Minute
	metadataFallbackUA    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)

type metadataSyncer struct {
	playback   *playback.Service
	failures   *FailureCollector
	client     *http.Client
	onProgress ScanProgressReporter
}

type metadataItem struct {
	fileID        string
	fileName      string
	relDirs       []string
	relPath       string
	legacyRelPath string
	direct        bool
}

func (m *metadataSyncer) syncFiles(ctx context.Context, accountID int64, root string, items []metadataItem) (int64, error) {
	if m == nil || m.playback == nil || len(items) == 0 {
		return 0, nil
	}
	pending := pendingMetadataItems(root, items, m.failures)
	if len(pending) == 0 {
		return 0, nil
	}
	total := len(pending)
	reportMetadataProgress(m.onProgress, 0, total, "")
	client := m.client
	if client == nil {
		client = &http.Client{Timeout: metadataClientTimeout}
	}
	var created int64
	for i, item := range pending {
		if err := ctx.Err(); err != nil {
			return created, err
		}
		label := metadataProgressLabel(item.relPath)
		reportMetadataProgress(m.onProgress, i, total, label)
		ok, err := m.syncOne(ctx, client, accountID, root, item)
		if err != nil {
			return created, err
		}
		if ok {
			created++
		}
		reportMetadataProgress(m.onProgress, i+1, total, label)
	}
	return created, nil
}

func filterPendingMetadataItems(root string, items []metadataItem) []metadataItem {
	return pendingMetadataItems(root, items, nil)
}

func pendingMetadataItems(root string, items []metadataItem, failures *FailureCollector) []metadataItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]metadataItem, 0, len(items))
	for _, item := range items {
		dest := filepath.Join(root, item.relPath)
		if pathHasOversizedComponent(dest) {
			addOversizedPathFailure(failures, ScanFailureMetadata, item.relPath, false)
			continue
		}
		if info, statErr := os.Stat(dest); statErr == nil && info.Size() > 0 {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (m *metadataSyncer) syncOne(ctx context.Context, client *http.Client, accountID int64, root string, item metadataItem) (created bool, err error) {
	dest := filepath.Join(root, item.relPath)
	if pathHasOversizedComponent(dest) {
		addOversizedPathFailure(m.failures, ScanFailureMetadata, item.relPath, false)
		return false, nil
	}
	if info, statErr := os.Stat(dest); statErr == nil && info.Size() > 0 {
		return false, nil
	}
	if item.legacyRelPath != "" {
		legacy := filepath.Join(root, item.legacyRelPath)
		if info, statErr := os.Stat(legacy); statErr == nil && info.Size() > 0 {
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				m.recordFailure(item.relPath, err.Error())
				return false, nil
			}
			if err := os.Rename(legacy, dest); err != nil {
				m.recordFailure(item.relPath, err.Error())
				return false, nil
			}
			return true, nil
		}
	}
	body, dlErr := m.downloadWithRetry(ctx, client, accountID, item.fileID, 0)
	if dlErr != nil {
		m.recordFailure(item.relPath, dlErr.Error())
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		m.recordFailure(item.relPath, err.Error())
		return false, nil
	}
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		m.recordFailure(item.relPath, err.Error())
		return false, nil
	}
	return true, nil
}

func (m *metadataSyncer) recordFailure(path, reason string) {
	if m.failures != nil {
		m.failures.Add(ScanFailureMetadata, path, reason)
	}
}

func (m *metadataSyncer) downloadWithRetry(ctx context.Context, client *http.Client, accountID int64, fileID string, expectedSize int64) ([]byte, error) {
	var lastErr error
	for round := 0; round < metadataResolveRounds; round++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		refresh := round > 0
		res, err := m.playback.Resolve(ctx, accountID, fileID, "", refresh)
		if err != nil {
			lastErr = err
			continue
		}
		if res.Link.URL == "" {
			lastErr = fmt.Errorf("无下载地址")
			continue
		}
		size := expectedSize
		if size <= 0 && res.File.Size > 0 {
			size = res.File.Size
		}
		body, fetchErr := fetchMetadataURLWithRetry(ctx, client, res.Link.URL, res.Link.Headers, size)
		if fetchErr == nil {
			return body, nil
		}
		lastErr = fetchErr
		if isIntegrityMetadataErr(fetchErr) || isRefreshMetadataErr(fetchErr) {
			continue
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("元数据下载失败")
	}
	return nil, lastErr
}

func fetchMetadataURLWithRetry(ctx context.Context, client *http.Client, downloadURL string, headers http.Header, expectedSize int64) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < metadataHTTPAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		body, err := fetchMetadataURLOnce(ctx, client, downloadURL, headers, expectedSize)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if isIntegrityMetadataErr(err) {
			return nil, err
		}
		if !isTransientMetadataErr(err) || attempt >= metadataHTTPAttempts-1 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("元数据下载失败")
	}
	return nil, lastErr
}

func prepareMetadataFetchHeaders(src http.Header) http.Header {
	h := src.Clone()
	if h == nil {
		h = make(http.Header)
	}
	if h.Get("Accept") == "" {
		h.Set("Accept", "*/*")
	}
	if h.Get("User-Agent") == "" {
		h.Set("User-Agent", metadataFallbackUA)
	}
	h.Set("Accept-Encoding", "identity")
	h.Set("Connection", "close")
	h.Del("Cache-Control")
	h.Del("Range")
	return h
}

func fetchMetadataURLOnce(ctx context.Context, client *http.Client, downloadURL string, headers http.Header, expectedSize int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	for k, vs := range prepareMetadataFetchHeaders(headers) {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 && resp.StatusCode <= 504 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if cl := strings.TrimSpace(resp.Header.Get("Content-Length")); cl != "" {
		if want, convErr := strconv.Atoi(cl); convErr == nil && len(data) != want {
			return nil, fmt.Errorf("Content-Length不一致: expected=%d, got=%d", want, len(data))
		}
	}
	if expectedSize > 0 && int64(len(data)) != expectedSize {
		return nil, fmt.Errorf("文件大小不一致: expected=%d, got=%d", expectedSize, len(data))
	}
	return data, nil
}

func isTransientMetadataErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "connection reset") || strings.Contains(msg, "broken pipe") {
		return true
	}
	if strings.HasPrefix(msg, "http 5") {
		return true
	}
	return false
}

func isIntegrityMetadataErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "不一致")
}

func isRefreshMetadataErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "invalid signature") {
		return true
	}
	if strings.Contains(msg, "http 403") || strings.Contains(msg, "http 401") {
		return true
	}
	return false
}

func metadataRelPath(outputFolder string, relDirs []string, fileName string) string {
	parts := make([]string, 0, len(relDirs)+2)
	parts = append(parts, SafeName(outputFolder))
	for _, dir := range relDirs {
		parts = append(parts, SafeName(dir))
	}
	parts = append(parts, SafeName(fileName))
	return filepath.Join(parts...)
}

func newMetadataItem(fileID, fileName, outputFolder string, relDirs []string) metadataItem {
	return metadataItem{
		fileID:   fileID,
		fileName: fileName,
		relDirs:  append([]string{}, relDirs...),
		relPath:  metadataRelPath(outputFolder, relDirs, fileName),
		direct:   true,
	}
}

func alignMetadataItems(outputFolder string, media []mediaCandidate, items []metadataItem, isoFilenameEnabled bool) []metadataItem {
	if len(items) == 0 || !isoFilenameEnabled {
		return items
	}
	isoStems := make(map[string][]string)
	for _, item := range media {
		if !isISOFileName(item.fileName) {
			continue
		}
		key := dirKey(item.relDirs)
		isoStems[key] = append(isoStems[key], MediaStem(item.fileName))
	}
	if len(isoStems) == 0 {
		return items
	}
	out := make([]metadataItem, len(items))
	for key := range isoStems {
		sort.SliceStable(isoStems[key], func(i, j int) bool {
			return len(isoStems[key][i]) > len(isoStems[key][j])
		})
	}
	for i, item := range items {
		out[i] = item
		for _, stem := range isoStems[dirKey(item.relDirs)] {
			if !hasMetadataStemPrefix(item.fileName, stem) {
				continue
			}
			alignedName, changed := alignISOMetadataName(item.fileName, stem)
			if changed {
				out[i].legacyRelPath = item.relPath
				out[i].relPath = metadataRelPath(outputFolder, item.relDirs, alignedName)
				out[i].direct = false
			}
			break
		}
	}
	return out
}

func hasMetadataStemPrefix(name, stem string) bool {
	prefix := stem + "."
	return len(name) > len(prefix) && strings.EqualFold(name[:len(prefix)], prefix)
}

func alignISOMetadataName(name, stem string) (string, bool) {
	prefix := stem + "."
	if len(name) <= len(prefix) || !strings.EqualFold(name[:len(prefix)], prefix) {
		return name, false
	}
	isoPrefix := stem + ".iso."
	if strings.HasPrefix(strings.ToLower(name), strings.ToLower(isoPrefix)) {
		return name, false
	}
	return name[:len(stem)] + ".iso" + name[len(stem):], true
}
