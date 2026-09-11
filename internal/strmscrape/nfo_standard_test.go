package strmscrape

import (
	"os"
	"path/filepath"
	"testing"
)

// 压制组随片发布的 MediaInfo 文本，会被元数据同步复制成 .nfo，但不是标准 NFO。
const sceneMediaInfoNFO = `THEATRE DATE....: 2023
iMDB URL........: http://www.imdb.com/title/tt22488024/
GENRE...........: Action, Adventure
ViDEO BiTRATE...: x265 L5 Main 10 @ 27.5 Mbps
FilE SiZE.......: 34.33 G
`

func TestNFOLooksStandard(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("写文件失败: %v", err)
		}
		return path
	}

	if nfoLooksStandard(filepath.Join(dir, "不存在.nfo")) {
		t.Fatal("文件不存在不应算标准 NFO")
	}
	if nfoLooksStandard(write("scene.nfo", sceneMediaInfoNFO)) {
		t.Fatal("MediaInfo 文本不应算标准 NFO")
	}
	if !nfoLooksStandard(write("movie.nfo", "<?xml version=\"1.0\"?>\n<movie><title>A</title></movie>\n")) {
		t.Fatal("带 XML 声明的 movie 根应算标准 NFO")
	}
	if !nfoLooksStandard(write("upper.nfo", "<MOVIE><TITLE>A</TITLE></MOVIE>")) {
		t.Fatal("大写根节点应算标准 NFO")
	}
	if !nfoLooksStandard(write("tv.nfo", "<tvshow><title>S</title></tvshow>")) {
		t.Fatal("tvshow 根应算标准 NFO")
	}
	if nfoLooksStandard(write("ep.nfo", "<episodedetails><title>E1</title></episodedetails>")) {
		t.Fatal("分集 NFO 不是作品级 NFO")
	}
}

func TestNFOOverwriteNonStandard(t *testing.T) {
	year := 2023
	dir := t.TempDir()
	nfo := filepath.Join(dir, "天龙八部之乔峰传 (2023).nfo")
	if err := os.WriteFile(nfo, []byte(sceneMediaInfoNFO), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	// 非标准 NFO 直接重写（覆盖），不保留备份。
	if !nfoWriteNeeded(false, nfo) {
		t.Fatal("非标准 NFO 应重写")
	}
	if err := writeMovieNFO(nfo, "天龙八部之乔峰传", "22488024", "剧情", &year); err != nil {
		t.Fatalf("写 NFO 失败: %v", err)
	}
	if !nfoLooksStandard(nfo) {
		t.Fatal("重写后应为标准 NFO")
	}
	if fileExists(nfo + ".scene.bak") {
		t.Fatal("不应保留备份文件")
	}
	// 已是标准 NFO 且非覆盖模式：不再重写。
	if nfoWriteNeeded(false, nfo) {
		t.Fatal("标准 NFO 且非覆盖模式不应重写")
	}
	if !nfoWriteNeeded(true, nfo) {
		t.Fatal("覆盖模式应重写")
	}
}

func TestSceneNFOIsNotTreatedAsMetadata(t *testing.T) {
	g := newCompleteMovieWork(t)
	// 用压制组信息文件替换标准 NFO，模拟「元数据同步」把网盘上的 .nfo 覆盖成了非标准文件。
	nfo := filepath.Join(g.absDir, "阿凡达 (2009).nfo")
	if err := os.WriteFile(nfo, []byte(sceneMediaInfoNFO), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	if workHasNFO(g, MediaTypeMovie) {
		t.Fatal("非标准 .nfo 不应被当作已有元数据")
	}
	if workHasActors(g, MediaTypeMovie) {
		t.Fatal("非标准 .nfo 不应参与演员判定")
	}
	if !workNeedsScrape(g, MediaTypeMovie, Settings{}) {
		t.Fatal("缺标准 NFO 时应重新刮削（会直接覆盖非标准文件）")
	}
	if !workHasPoster(g, MediaTypeMovie) {
		t.Fatal("海报不受影响")
	}
}
