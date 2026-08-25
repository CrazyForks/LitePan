package strmscrape

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	pendingMarkerName        = ".litepan-scrape-pending"
	manualCompleteMarkerName = ".litepan-scrape-complete"
	ownedMetadataMarkerName  = ".litepan-scrape-owned"

	PendingRunning    = "running"
	PendingUpdating   = "updating"
	PendingIncomplete = "incomplete"
	PendingDoubt      = "doubt"

	TVStateEnded    = "ended"
	TVStateUpdating = "updating"
)

// scrapeState 只落在 .litepan-scrape-pending；完结删除该文件。
type scrapeState struct {
	Status  string `json:"status,omitempty"` // running|updating|incomplete|doubt
	EpLocal int    `json:"ep_local,omitempty"`
	EpTMDB  int    `json:"ep_tmdb,omitempty"`
}

// manualCompleteState 表示用户确认该作品无需继续匹配 TMDB。
// 独立标记不能再通过“缺少 pending”推断，否则没有 NFO/海报的本地作品会反复进入待刮削。
type manualCompleteState struct {
	MediaType string `json:"media_type,omitempty"`
}

// ownedMetadataState 只登记 STRM 刮削器实际写入的文件。
// 取消错误匹配时据此清理，避免删除用户从网盘同步或自行维护的元数据。
type ownedMetadataState struct {
	Files []string `json:"files"`
}

func workMarkerPath(g workGroup, name string) string {
	if g.flatFile != "" {
		stem := strings.TrimSuffix(g.flatFile, filepath.Ext(g.flatFile))
		return stem + name
	}
	return filepath.Join(g.absDir, name)
}

func pendingMarkerPath(g workGroup) string {
	return workMarkerPath(g, pendingMarkerName)
}

func manualCompleteMarkerPath(g workGroup) string {
	return workMarkerPath(g, manualCompleteMarkerName)
}

func ownedMetadataMarkerPath(g workGroup) string {
	return workMarkerPath(g, ownedMetadataMarkerName)
}

func hasPendingMarker(g workGroup) bool {
	return fileExists(pendingMarkerPath(g))
}

func clearPendingMarker(g workGroup) {
	_ = os.Remove(pendingMarkerPath(g))
}

func writeManualComplete(g workGroup, mediaType string) error {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType != MediaTypeTV && mediaType != MediaTypeMovie {
		mediaType = inferMediaType(g)
	}
	data, err := json.Marshal(manualCompleteState{MediaType: mediaType})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := writeMarkerFile(manualCompleteMarkerPath(g), data); err != nil {
		return err
	}
	clearPendingMarker(g)
	return nil
}

func readManualComplete(g workGroup) (manualCompleteState, bool) {
	data, err := os.ReadFile(manualCompleteMarkerPath(g))
	if err != nil {
		return manualCompleteState{}, false
	}
	var st manualCompleteState
	if json.Unmarshal(data, &st) != nil {
		return manualCompleteState{MediaType: inferMediaType(g)}, true
	}
	return st, true
}

func clearManualComplete(g workGroup) {
	_ = os.Remove(manualCompleteMarkerPath(g))
}

func recordOwnedMetadata(g workGroup, path string) error {
	rel, ok := ownedMetadataRel(g, path)
	if !ok {
		return nil
	}
	state := ownedMetadataState{}
	if data, err := os.ReadFile(ownedMetadataMarkerPath(g)); err == nil {
		_ = json.Unmarshal(data, &state)
	}
	for _, existing := range state.Files {
		if existing == rel {
			return nil
		}
	}
	state.Files = append(state.Files, rel)
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeMarkerFile(ownedMetadataMarkerPath(g), data)
}

func clearOwnedMetadata(g workGroup) error {
	data, err := os.ReadFile(ownedMetadataMarkerPath(g))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var state ownedMetadataState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	for _, rel := range state.Files {
		path, ok := ownedMetadataPath(g, rel)
		if !ok {
			continue
		}
		info, statErr := os.Lstat(path)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return statErr
		}
		if info.IsDir() || strings.EqualFold(filepath.Ext(path), ".strm") {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Remove(ownedMetadataMarkerPath(g))
}

func ownedMetadataRel(g workGroup, path string) (string, bool) {
	base, err := filepath.Abs(g.absDir)
	if err != nil {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(base, abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func ownedMetadataPath(g workGroup, rel string) (string, bool) {
	rel = filepath.FromSlash(strings.TrimSpace(rel))
	if rel == "" || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	path := filepath.Join(g.absDir, rel)
	_, ok := ownedMetadataRel(g, path)
	return path, ok
}

func writePendingState(g workGroup, st scrapeState) error {
	if st.Status == "" {
		st.Status = PendingRunning
	}
	return writeJSONMarker(pendingMarkerPath(g), st)
}

func writePendingMarker(g workGroup) error {
	return writePendingState(g, scrapeState{Status: PendingRunning})
}

func readPendingState(g workGroup) (scrapeState, bool) {
	return readJSONMarker(pendingMarkerPath(g))
}

func writeMarkerFile(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func writeJSONMarker(path string, st scrapeState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeMarkerFile(path, data)
}

func readJSONMarker(path string) (scrapeState, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return scrapeState{}, false
	}
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "pending" {
		return scrapeState{Status: PendingRunning}, true
	}
	var st scrapeState
	if json.Unmarshal([]byte(raw), &st) != nil {
		return scrapeState{Status: PendingRunning}, true
	}
	if st.Status == "" {
		st.Status = PendingRunning
	}
	return st, true
}

// finalizeAfterScrape：按集数/存疑决定保留或删除 pending，并写回 ep_local/ep_tmdb。
func finalizeAfterScrape(g workGroup, mediaType string, epTMDB int, doubt bool) {
	epLocal, epScraped := countTVEpisodeProgress(g)
	st := scrapeState{EpLocal: epLocal, EpTMDB: epTMDB}
	if doubt {
		st.Status = PendingDoubt
		_ = writePendingState(g, st)
		return
	}
	if mediaType != MediaTypeTV || g.flatFile != "" {
		clearPendingMarker(g)
		return
	}
	if epTMDB > 0 && epLocal < epTMDB {
		st.Status = PendingUpdating
		_ = writePendingState(g, st)
		return
	}
	if epTMDB > 0 && epLocal > epTMDB {
		st.Status = PendingIncomplete
		_ = writePendingState(g, st)
		return
	}
	if epLocal > 0 && epScraped < epLocal {
		st.Status = PendingIncomplete
		_ = writePendingState(g, st)
		return
	}
	clearPendingMarker(g)
}

// markWorkNormal：根已齐时清除 pending（设为完结，短剧等不再追分集）。
func markWorkNormal(g workGroup, mediaType string) error {
	if !workHasNFO(g, mediaType) || !workHasPoster(g, mediaType) {
		return errRootMetaIncomplete
	}
	clearPendingMarker(g)
	return nil
}
