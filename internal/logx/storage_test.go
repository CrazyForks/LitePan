package logx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStorageQueryPaginatesNewestFirstAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	writeLogEntries(t, dir, "2026-07-18.log", []Entry{
		{Timestamp: "2026-07-18T08:00:00+08:00", Level: LevelInfo, Module: "system", Message: "old-info"},
		{Timestamp: "2026-07-18T09:00:00+08:00", Level: LevelError, Module: "system", Message: "old-error"},
	})
	writeLogEntries(t, dir, "2026-07-19.log", []Entry{
		{Timestamp: "2026-07-19T08:00:00+08:00", Level: LevelInfo, Module: "system", Message: "new-info"},
		{Timestamp: "2026-07-19T09:00:00+08:00", Level: LevelError, Module: "system", Message: "new-error"},
		{Timestamp: "2026-07-19T10:00:00+08:00", Level: LevelInfo, Module: "system", Message: "newest"},
	})

	storage := &Storage{dir: dir}
	minLevel := LevelInfo
	got, err := storage.Query(QueryFilter{MinLevel: &minLevel, Limit: 3, Offset: 1})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if messages := entryMessages(got); !reflect.DeepEqual(messages, []string{"new-error", "new-info", "old-error"}) {
		t.Fatalf("Query() messages = %v", messages)
	}

	errorLevel := LevelError
	got, err = storage.Query(QueryFilter{Level: &errorLevel, Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("Query(error) error = %v", err)
	}
	if messages := entryMessages(got); !reflect.DeepEqual(messages, []string{"old-error"}) {
		t.Fatalf("Query(error) messages = %v", messages)
	}
}

func TestScanLinesReverseStopsBeforeReadingEarlierLongLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-07-19.log")
	writeLogEntries(t, dir, filepath.Base(path), []Entry{
		{
			Timestamp: "2026-07-19T09:00:00+08:00",
			Level:     LevelInfo,
			Module:    "system",
			Message:   strings.Repeat("x", reverseReadBlockSize*2),
		},
		{Timestamp: "2026-07-19T10:00:00+08:00", Level: LevelInfo, Module: "system", Message: "newest"},
	})

	var messages []string
	stopped, err := scanLinesReverse(path, func(line []byte) bool {
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		messages = append(messages, entry.Message)
		return false
	})
	if err != nil {
		t.Fatalf("scanLinesReverse() error = %v", err)
	}
	if !stopped {
		t.Fatal("scanLinesReverse() stopped = false")
	}
	if !reflect.DeepEqual(messages, []string{"newest"}) {
		t.Fatalf("scanLinesReverse() messages = %v", messages)
	}

	messages = nil
	stopped, err = scanLinesReverse(path, func(line []byte) bool {
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("json.Unmarshal(full scan) error = %v", err)
		}
		messages = append(messages, entry.Message)
		return true
	})
	if err != nil {
		t.Fatalf("scanLinesReverse(full scan) error = %v", err)
	}
	if stopped {
		t.Fatal("scanLinesReverse(full scan) stopped = true")
	}
	if len(messages) != 2 || messages[0] != "newest" || len(messages[1]) != reverseReadBlockSize*2 {
		t.Fatalf("scanLinesReverse(full scan) message lengths = %v", messageLengths(messages))
	}
}

func TestStatsFilteredCacheIsClonedAndCleanupInvalidatesIt(t *testing.T) {
	dir := t.TempDir()
	writeLogEntries(t, dir, "2026-07-19.log", []Entry{
		{Timestamp: "2026-07-19T09:00:00+08:00", Level: LevelInfo, Module: "system", Message: "one"},
		{Timestamp: "2026-07-19T10:00:00+08:00", Level: LevelInfo, Module: "system", Message: "two"},
	})
	storage := &Storage{dir: dir}

	first := storage.StatsFiltered(LevelInfo)
	if first.Total != 2 {
		t.Fatalf("StatsFiltered().Total = %d", first.Total)
	}
	first.ByLevel["INFO"] = 99
	if cached := storage.StatsFiltered(LevelInfo); cached.ByLevel["INFO"] != 2 {
		t.Fatalf("cached ByLevel[INFO] = %d", cached.ByLevel["INFO"])
	}

	if _, err := storage.ClearAllLogs(); err != nil {
		t.Fatalf("ClearAllLogs() error = %v", err)
	}
	if afterCleanup := storage.StatsFiltered(LevelInfo); afterCleanup.Total != 0 {
		t.Fatalf("StatsFiltered() after cleanup total = %d", afterCleanup.Total)
	}
}

func writeLogEntries(t *testing.T, dir, name string, entries []Entry) {
	t.Helper()
	var data []byte
	for _, entry := range entries {
		line, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
}

func entryMessages(entries []Entry) []string {
	messages := make([]string, 0, len(entries))
	for _, entry := range entries {
		messages = append(messages, entry.Message)
	}
	return messages
}

func messageLengths(messages []string) []int {
	lengths := make([]int, 0, len(messages))
	for _, message := range messages {
		lengths = append(lengths, len(message))
	}
	return lengths
}
