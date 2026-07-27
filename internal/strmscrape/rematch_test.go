package strmscrape

import (
	"context"
	"path/filepath"
	"testing"

	"litepan/internal/domain"
	"litepan/internal/strm"
)

type rematchTaskRepo struct {
	task *domain.StrmTask
}

func (r *rematchTaskRepo) Create(context.Context, *domain.StrmTask) (int64, error) { return 0, nil }
func (r *rematchTaskRepo) Update(context.Context, *domain.StrmTask) error          { return nil }
func (r *rematchTaskRepo) Delete(context.Context, int64) error                     { return nil }
func (r *rematchTaskRepo) Get(context.Context, int64) (*domain.StrmTask, error)    { return r.task, nil }
func (r *rematchTaskRepo) List(context.Context) ([]*domain.StrmTask, error) {
	return []*domain.StrmTask{r.task}, nil
}
func (r *rematchTaskRepo) ListByAccount(context.Context, int64) ([]*domain.StrmTask, error) {
	return []*domain.StrmTask{r.task}, nil
}
func (r *rematchTaskRepo) UpdateScan(context.Context, int64, domain.StrmScanPatch) error {
	return nil
}

func TestConfirmExistingMatchClearsDoubtWhenMetadataComplete(t *testing.T) {
	root := t.TempDir()
	show := filepath.Join(root, "海贼王 (1999)")
	season := filepath.Join(show, "Season 01")
	mustMkdir(t, season)
	mustWrite(t, filepath.Join(season, "海贼王.S01E01.strm"), "x")
	mustWrite(t, filepath.Join(season, "海贼王.S01E01.nfo"), "<episodedetails></episodedetails>\n")
	mustWrite(t, filepath.Join(show, "tvshow.nfo"), "<tvshow><title>航海王</title><year>1999</year><tmdbid>37854</tmdbid></tvshow>\n")
	mustWrite(t, filepath.Join(show, "poster.jpg"), "image")

	works, err := scanWorks(root)
	if err != nil {
		t.Fatal(err)
	}
	g := works[0]
	if err := writePendingState(g, scrapeState{Status: PendingDoubt, EpLocal: 1, EpTMDB: 1}); err != nil {
		t.Fatal(err)
	}
	if !workTMDBIDMatches(g, MediaTypeTV, "37854") {
		t.Fatal("相同 TMDB ID 未被识别")
	}

	confirmExistingMatch(g, MediaTypeTV)
	if _, ok := readPendingState(g); ok {
		t.Fatal("确认相同匹配后应清除存疑状态")
	}
}

func TestConfirmExistingMatchKeepsUpdatingState(t *testing.T) {
	root := t.TempDir()
	show := filepath.Join(root, "追更剧 (2026)")
	season := filepath.Join(show, "Season 01")
	mustMkdir(t, season)
	mustWrite(t, filepath.Join(season, "追更剧.S01E01.strm"), "x")
	mustWrite(t, filepath.Join(season, "追更剧.S01E01.nfo"), "<episodedetails></episodedetails>\n")
	mustWrite(t, filepath.Join(show, "tvshow.nfo"), "<tvshow><title>追更剧</title><tmdbid>1</tmdbid></tvshow>\n")
	mustWrite(t, filepath.Join(show, "poster.jpg"), "image")

	works, err := scanWorks(root)
	if err != nil {
		t.Fatal(err)
	}
	g := works[0]
	if err := writePendingState(g, scrapeState{Status: PendingUpdating, EpLocal: 1, EpTMDB: 2}); err != nil {
		t.Fatal(err)
	}
	confirmExistingMatch(g, MediaTypeTV)
	pending, ok := readPendingState(g)
	if !ok || pending.Status != PendingUpdating {
		t.Fatalf("确认匹配不能误清除追更状态，实际=%+v", pending)
	}
}

func TestMarkNormalConfirmsDoubtAndKeepsUpdatingState(t *testing.T) {
	strmRoot := t.TempDir()
	outputFolder := "123剧集"
	root := strm.TaskOutputDir(strmRoot, outputFolder)
	show := filepath.Join(root, "追更剧 (2026)")
	season := filepath.Join(show, "Season 01")
	mustMkdir(t, season)
	mustWrite(t, filepath.Join(season, "追更剧.S01E01.strm"), "x")
	mustWrite(t, filepath.Join(season, "追更剧.S01E01.nfo"), "<episodedetails></episodedetails>\n")
	mustWrite(t, filepath.Join(show, "tvshow.nfo"), "<tvshow><title>追更剧</title><tmdbid>1</tmdbid></tvshow>\n")
	mustWrite(t, filepath.Join(show, "poster.jpg"), "image")

	works, err := scanWorks(root)
	if err != nil {
		t.Fatal(err)
	}
	g := works[0]
	if err := writePendingState(g, scrapeState{Status: PendingDoubt, EpLocal: 1, EpTMDB: 2}); err != nil {
		t.Fatal(err)
	}
	task := &domain.StrmTask{ID: 8, OutputFolder: outputFolder}
	strmSvc := strm.NewService(strm.ServiceOptions{
		Repo:    &rematchTaskRepo{task: task},
		StrmDir: strmRoot,
	})
	svc := New(Options{Strm: strmSvc, StrmDir: strmRoot, DataDir: t.TempDir()})

	item, err := svc.MarkNormal(context.Background(), MarkNormalRequest{
		StrmTaskID: 8,
		ItemID:     pathToItemID(g.relKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != ItemStatusOK || item.TVState != TVStateUpdating || !item.HasPending {
		t.Fatalf("确认存疑后应恢复正常但保持追更，实际=%+v", item)
	}
}

func TestRematchSameCompleteIDRequiresTMDBClient(t *testing.T) {
	strmRoot := t.TempDir()
	outputFolder := "123电影"
	root := strm.TaskOutputDir(strmRoot, outputFolder)
	show := filepath.Join(root, "动漫剧", "海贼王 (1999)")
	season := filepath.Join(show, "Season 01")
	mustMkdir(t, season)
	mustWrite(t, filepath.Join(season, "海贼王.S01E01.strm"), "x")
	mustWrite(t, filepath.Join(season, "海贼王.S01E01.nfo"), "<episodedetails></episodedetails>\n")
	mustWrite(t, filepath.Join(show, "tvshow.nfo"), "<tvshow><title>航海王</title><year>1999</year><tmdbid>37854</tmdbid></tvshow>\n")
	mustWrite(t, filepath.Join(show, "poster.jpg"), "image")

	works, err := scanWorks(root)
	if err != nil {
		t.Fatal(err)
	}
	g := works[0]
	if err := writePendingState(g, scrapeState{Status: PendingDoubt, EpLocal: 1, EpTMDB: 1}); err != nil {
		t.Fatal(err)
	}
	task := &domain.StrmTask{ID: 7, OutputFolder: outputFolder}
	strmSvc := strm.NewService(strm.ServiceOptions{
		Repo:    &rematchTaskRepo{task: task},
		StrmDir: strmRoot,
	})
	svc := New(Options{
		Strm:    strmSvc,
		StrmDir: strmRoot,
		DataDir: t.TempDir(),
	})

	_, _, err = svc.Rematch(context.Background(), RematchRequest{
		StrmTaskID: 7,
		ItemID:     pathToItemID(g.relKey),
		TMDBID:     "37854",
		MediaType:  MediaTypeTV,
		Title:      "航海王",
	})
	if err == nil {
		t.Fatal("重新匹配复用开始刮削路径，未配置 TMDB 时应失败")
	}
}

func TestOverwriteForMatchFollowsWriteMode(t *testing.T) {
	svc := New(Options{})
	if !svc.overwriteForMatch(false) {
		t.Fatal("换 TMDB ID 应强制覆盖")
	}
	if svc.overwriteForMatch(true) {
		t.Fatal("同 ID 且默认写入策略应为仅补缺")
	}
}

func TestRescrapeRequiresTMDBClient(t *testing.T) {
	strmRoot := t.TempDir()
	outputFolder := "123完结"
	root := strm.TaskOutputDir(strmRoot, outputFolder)
	show := filepath.Join(root, "海贼王 (1999)")
	season := filepath.Join(show, "Season 01")
	mustMkdir(t, season)
	mustWrite(t, filepath.Join(season, "海贼王.S01E01.strm"), "x")
	mustWrite(t, filepath.Join(season, "海贼王.S01E01.nfo"), "<episodedetails></episodedetails>\n")
	mustWrite(t, filepath.Join(show, "tvshow.nfo"), "<tvshow><title>航海王</title><year>1999</year><tmdbid>37854</tmdbid></tvshow>\n")
	mustWrite(t, filepath.Join(show, "poster.jpg"), "image")

	works, err := scanWorks(root)
	if err != nil {
		t.Fatal(err)
	}
	g := works[0]
	task := &domain.StrmTask{ID: 9, OutputFolder: outputFolder}
	strmSvc := strm.NewService(strm.ServiceOptions{
		Repo:    &rematchTaskRepo{task: task},
		StrmDir: strmRoot,
	})
	svc := New(Options{Strm: strmSvc, StrmDir: strmRoot, DataDir: t.TempDir()})

	_, _, err = svc.Rescrape(context.Background(), RescrapeRequest{
		StrmTaskID: 9,
		ItemID:     pathToItemID(g.relKey),
	})
	if err == nil {
		t.Fatal("重刮复用开始刮削路径，未配置 TMDB 时应失败")
	}
}
