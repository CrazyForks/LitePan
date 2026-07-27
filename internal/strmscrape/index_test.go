package strmscrape

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTaskIndexPathAndRemove(t *testing.T) {
	dir := t.TempDir()
	path := TaskIndexPath(dir, 42)
	want := filepath.Join(dir, "strmscrape", "42.sqlite")
	if path != want {
		t.Fatalf("path=%s want %s", path, want)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(path+"-wal", []byte("w"), 0o644)
	RemoveTaskIndex(dir, 42)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("index should be removed, err=%v", err)
	}
}

func TestIndexUpsertAndList(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{dataDir: dir}
	path := svc.indexPath(9)
	db, err := openTaskIndexDB(path)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	year := 2021
	it := Item{
		ID: "abc", Title: "天龙八部", Year: &year, MediaType: MediaTypeTV,
		Status: ItemStatusOK, HasNFO: true, HasPoster: true, HasPending: true,
		TMDBID: "1", FolderName: "天龙八部 (2021)", FileCount: 2,
		EpLocal: 1, EpTMDB: 40, TVState: TVStateUpdating, AddedAt: "2026-01-01T00:00:00Z",
	}
	if err := upsertItemTx(tx, it, "poster.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := writeIndexMeta(tx, "schema", indexSchemaVersion); err != nil {
		t.Fatal(err)
	}
	if err := writeIndexMeta(tx, "root", "/tmp/out"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	items, err := svc.listIndexItems(9)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("len=%d", len(items))
	}
	got := items[0]
	if got.Title != "天龙八部" || got.EpTMDB != 40 || !got.HasPending {
		t.Fatalf("got=%+v", got)
	}
	if got.Year == nil || *got.Year != 2021 {
		t.Fatalf("year=%v", got.Year)
	}
	if got.PosterURL == "" || !got.HasPoster {
		t.Fatalf("poster url empty: %s", got.PosterURL)
	}
}
