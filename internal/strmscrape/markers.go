package strmscrape

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const (
	pendingMarkerName = ".litepan-scrape-pending"

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

func hasPendingMarker(g workGroup) bool {
	return fileExists(pendingMarkerPath(g))
}

func clearPendingMarker(g workGroup) {
	_ = os.Remove(pendingMarkerPath(g))
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
