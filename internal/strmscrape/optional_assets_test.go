package strmscrape

import (
	"os"
	"path/filepath"
	"testing"
)

// 构造一个「根已齐」的电影作品目录：<strm>.nfo + poster.jpg。
func newCompleteMovieWork(t *testing.T) workGroup {
	t.Helper()
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("写文件失败: %v", err)
		}
	}
	write("阿凡达 (2009).strm")
	if err := os.WriteFile(filepath.Join(dir, "阿凡达 (2009).nfo"),
		[]byte("<movie><title>阿凡达</title></movie>\n"), 0o644); err != nil {
		t.Fatalf("写 NFO 失败: %v", err)
	}
	write("poster.jpg")
	strmPath := filepath.Join(dir, "阿凡达 (2009).strm")
	return workGroup{
		relKey:  "电影/阿凡达 (2009)",
		absDir:  dir,
		entries: []strmEntry{{absPath: strmPath}},
	}
}

func allOptionalEnabled() Settings {
	return Settings{Fanart: true, ClearLogo: true, Actors: true, EpisodeInfo: true}
}

// writeDoneState 模拟「已刮削完结、TMDB 确认缺少全部可选资源」的落盘状态。
func writeDoneState(t *testing.T, g workGroup) {
	t.Helper()
	err := writePendingState(g, scrapeState{
		Status:     PendingDone,
		NoBackdrop: true,
		NoLogo:     true,
		NoActors:   true,
	})
	if err != nil {
		t.Fatalf("写状态失败: %v", err)
	}
}

func TestWorkNeedsScrapeRespectsMissingOptionalAssets(t *testing.T) {
	t.Run("TMDB 已确认没有则可选资源不再重刮", func(t *testing.T) {
		g := newCompleteMovieWork(t)
		if !workNeedsScrape(g, MediaTypeMovie, allOptionalEnabled()) {
			t.Fatalf("未记录结论时应因缺背景图/Logo/演员而需要刮削")
		}
		writeDoneState(t, g)
		if workNeedsScrape(g, MediaTypeMovie, allOptionalEnabled()) {
			t.Fatalf("已确认 TMDB 缺失时不应再刮削")
		}
	})

	t.Run("开关关闭时不因该资源重刮", func(t *testing.T) {
		g := newCompleteMovieWork(t)
		cfg := Settings{Fanart: false, ClearLogo: false, Actors: false}
		if workNeedsScrape(g, MediaTypeMovie, cfg) {
			t.Fatalf("三个可选开关都关闭时，根齐全即应跳过")
		}
	})

	t.Run("本地已有资源则忽略结论", func(t *testing.T) {
		g := newCompleteMovieWork(t)
		if err := os.WriteFile(filepath.Join(g.absDir, "fanart.jpg"), []byte("x"), 0o644); err != nil {
			t.Fatalf("写背景图失败: %v", err)
		}
		if err := os.WriteFile(filepath.Join(g.absDir, "clearlogo.png"), []byte("x"), 0o644); err != nil {
			t.Fatalf("写 Logo 失败: %v", err)
		}
		if err := os.WriteFile(filepath.Join(g.absDir, "阿凡达 (2009).nfo"), []byte("<movie><actor><name>A</name></actor></movie>"), 0o644); err != nil {
			t.Fatalf("写 NFO 失败: %v", err)
		}
		writeDoneState(t, g)
		if workNeedsScrape(g, MediaTypeMovie, allOptionalEnabled()) {
			t.Fatalf("资源齐全后不应再刮削")
		}
	})

	t.Run("只记一项时其余项仍会触发", func(t *testing.T) {
		g := newCompleteMovieWork(t)
		if err := writePendingState(g, scrapeState{Status: PendingDone, NoLogo: true}); err != nil {
			t.Fatalf("写状态失败: %v", err)
		}
		if !workNeedsScrape(g, MediaTypeMovie, allOptionalEnabled()) {
			t.Fatalf("未记录结论的背景图缺失仍应触发刮削")
		}
		if err := writePendingState(g, scrapeState{Status: PendingDone, NoLogo: true, NoBackdrop: true}); err != nil {
			t.Fatalf("写状态失败: %v", err)
		}
		if !workNeedsScrape(g, MediaTypeMovie, allOptionalEnabled()) {
			t.Fatalf("未记录结论的演员缺失仍应触发刮削")
		}
	})

	t.Run("done 不视为待刮削", func(t *testing.T) {
		g := newCompleteMovieWork(t)
		writeDoneState(t, g)
		if hasPendingMarker(g) {
			t.Fatalf("done 状态不应被当作 pending")
		}
		item := buildItem(1, g.absDir, g)
		if item.HasPending {
			t.Fatalf("done 状态不应上报 HasPending")
		}
		if item.Status != ItemStatusOK {
			t.Fatalf("根已齐且 done 时应为 OK，实际 %q", item.Status)
		}
	})

	t.Run("真实 pending 仍然要刮", func(t *testing.T) {
		g := newCompleteMovieWork(t)
		if err := writePendingState(g, scrapeState{Status: PendingDoubt}); err != nil {
			t.Fatalf("写状态失败: %v", err)
		}
		if !workNeedsScrape(g, MediaTypeMovie, Settings{}) {
			t.Fatalf("存疑状态应继续刮削")
		}
	})

	t.Run("手动完成标记仍优先", func(t *testing.T) {
		g := newCompleteMovieWork(t)
		if err := writeManualComplete(g, MediaTypeMovie); err != nil {
			t.Fatalf("写手动完成标记失败: %v", err)
		}
		if workNeedsScrape(g, MediaTypeMovie, allOptionalEnabled()) {
			t.Fatalf("手动标记无需匹配时不应刮削")
		}
	})
}

func TestSyncOptionalAssetState(t *testing.T) {
	t.Run("TMDB 无资源时写入 done 结论", func(t *testing.T) {
		g := newCompleteMovieWork(t)
		syncOptionalAssetState(g, allOptionalEnabled(), tmdbInfo{TMDBID: "76600"}, false)
		st, ok := readPendingState(g)
		if !ok || st.Status != PendingDone {
			t.Fatalf("应写入 done 状态，实际 %+v ok=%v", st, ok)
		}
		if !st.NoBackdrop || !st.NoLogo || !st.NoActors {
			t.Fatalf("应记录三项缺失，实际 %+v", st)
		}
		if hasPendingMarker(g) {
			t.Fatalf("done 不应表现为 pending")
		}
	})

	t.Run("TMDB 有资源时删除文件", func(t *testing.T) {
		g := newCompleteMovieWork(t)
		writeDoneState(t, g)
		info := tmdbInfo{TMDBID: "76600", BackdropPath: "/b.jpg", LogoPath: "/l.png"}
		info.Actors = []tmdbActor{{Name: "Sam Worthington"}}
		syncOptionalAssetState(g, allOptionalEnabled(), info, false)
		if _, ok := readPendingState(g); ok {
			t.Fatalf("无缺失结论时不应保留标记文件")
		}
	})

	t.Run("开关关闭时不写入该资源结论", func(t *testing.T) {
		g := newCompleteMovieWork(t)
		syncOptionalAssetState(g, Settings{Actors: true}, tmdbInfo{TMDBID: "76600"}, false)
		st, ok := readPendingState(g)
		if !ok {
			t.Fatalf("应写入标记")
		}
		if st.NoBackdrop || st.NoLogo {
			t.Fatalf("未检查的资源不应被记录，实际 %+v", st)
		}
		if !st.NoActors {
			t.Fatalf("开启的演员项缺失应被记录，实际 %+v", st)
		}
	})

	t.Run("追更中的 pending 不被改写", func(t *testing.T) {
		g := newCompleteMovieWork(t)
		if err := writePendingState(g, scrapeState{Status: PendingUpdating, EpLocal: 3, EpTMDB: 10}); err != nil {
			t.Fatalf("写状态失败: %v", err)
		}
		syncOptionalAssetState(g, allOptionalEnabled(), tmdbInfo{TMDBID: "76600"}, false)
		st, _ := readPendingState(g)
		if st.Status != PendingUpdating || st.EpTMDB != 10 {
			t.Fatalf("追更状态不应被覆盖，实际 %+v", st)
		}
		if st.NoBackdrop {
			t.Fatalf("追更未完结时不应写入可选资源结论，实际 %+v", st)
		}
	})
}

func TestMarkNormalStopsAutoScrape(t *testing.T) {
	root := t.TempDir()
	show := filepath.Join(root, "开局女帝盯上了我的彩礼 (2024)")
	s1 := filepath.Join(show, "Season 01")
	mustMkdir(t, s1)
	mustWrite(t, filepath.Join(s1, "开局女帝.S01E01.strm"), "x")
	mustWrite(t, filepath.Join(show, "tvshow.nfo"), "<tvshow><title>开局女帝</title></tvshow>\n")
	mustWrite(t, filepath.Join(show, "poster.jpg"), "img")
	works, err := scanWorks(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg := allOptionalEnabled()
	// 未设为完结：根齐但缺背景图/Logo/演员 → 仍需刮削。
	if !workNeedsScrape(works[0], MediaTypeTV, cfg) {
		t.Fatalf("根齐但可选资源缺失时应刮削")
	}
	if err := markWorkNormal(works[0], MediaTypeTV); err != nil {
		t.Fatalf("设为完结失败: %v", err)
	}
	if hasPendingMarker(works[0]) {
		t.Fatal("ended 不应被当作 pending")
	}
	if workNeedsScrape(works[0], MediaTypeTV, cfg) {
		t.Fatal("设为完结后不应再自动刮削（即使缺背景图/Logo/演员）")
	}
	item := buildItem(1, root, works[0])
	if item.Status != ItemStatusOK || item.TVState != TVStateEnded || item.HasPending {
		t.Fatalf("status=%s tv_state=%s has_pending=%v", item.Status, item.TVState, item.HasPending)
	}

	t.Run("手动重刮后按最新 TMDB 结果覆写终态", func(t *testing.T) {
		// TMDB 仍没有可选资源 → 记为 done 结论，之后不再重复刮削。
		syncOptionalAssetState(works[0], cfg, tmdbInfo{TMDBID: "1"}, false)
		if st, ok := readPendingState(works[0]); !ok || st.Status != PendingDone || !st.NoLogo {
			t.Fatalf("应写入 done + 缺失结论，实际 %+v ok=%v", st, ok)
		}
		if workNeedsScrape(works[0], MediaTypeTV, cfg) {
			t.Fatal("已记结论后不应再刮削")
		}
	})
}

func TestClearScrapedMetadataClearsState(t *testing.T) {
	g := newCompleteMovieWork(t)
	writeDoneState(t, g)
	if err := clearScrapedMetadata(g); err != nil {
		t.Fatalf("清理元数据失败: %v", err)
	}
	if _, ok := readPendingState(g); ok {
		t.Fatal("清理元数据后不应保留终态或结论")
	}
}
