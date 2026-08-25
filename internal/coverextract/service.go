package coverextract

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"litepan/internal/domain"
	"litepan/internal/driver"
	filecore "litepan/internal/file"
	"litepan/internal/mediaorganize/rules"
	"litepan/internal/playback"
)

const (
	maxFiles       = 20
	maxFrames      = 50
	maxImageBytes  = int64(200 << 20)
	defaultReadMax = int64(256 << 20)
	// MaxPosterBytes 限制前端 Canvas 合成后上传的海报大小。
	MaxPosterBytes = int64(8 << 20)
)

var supportedExt = map[string]struct{}{`.mp4`: {}, `.mkv`: {}, `.mov`: {}, `.webm`: {}}

type Options struct {
	DataDir    string
	ListenAddr string
	Files      *filecore.Service
	Playback   *playback.Service
}

type Service struct {
	mu         sync.Mutex
	files      map[string]*SessionFile
	frames     map[string]*imageEntry
	tokens     map[string]*sourceToken
	imageLen   int64
	sem        chan struct{}
	downloadMu sync.Mutex
	opts       Options
}

type SessionFile struct {
	ID             string  `json:"id"`
	AccountID      int64   `json:"account_id"`
	FileID         string  `json:"file_id"`
	ParentID       string  `json:"parent_id"`
	TargetParentID string  `json:"target_parent_id"`
	TargetPath     string  `json:"target_path"`
	Name           string  `json:"name"`
	Size           int64   `json:"size"`
	Status         string  `json:"status"`
	Error          string  `json:"error,omitempty"`
	DurationMS     int64   `json:"duration_ms,omitempty"`
	Frames         []Frame `json:"frames"`
	TouchedAt      int64   `json:"touched_at"`
}

type DirectoryRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Frame struct {
	ID     string `json:"id"`
	TimeMS int64  `json:"time_ms"`
}

type imageEntry struct {
	Data      []byte
	CreatedAt time.Time
}

type sourceToken struct {
	AccountID int64
	FileID    string
	ExpiresAt time.Time
	Read      int64
	MaxRead   int64
}

type ExtractRequest struct {
	SessionFileID string `json:"session_file_id"`
	Mode          string `json:"mode"`
	Count         int    `json:"count"`
	TimestampMS   int64  `json:"timestamp_ms"`
}

type SaveRequest struct {
	SessionFileID string `json:"session_file_id"`
	FrameID       string `json:"frame_id"`
	Overwrite     bool   `json:"overwrite"`
}

type SaveResult struct {
	OK       bool   `json:"ok"`
	Conflict bool   `json:"conflict,omitempty"`
	Filename string `json:"filename"`
}

func New(opts Options) (*Service, error) {
	if opts.Files == nil || opts.Playback == nil || strings.TrimSpace(opts.DataDir) == "" {
		return nil, errors.New("视频海报生成服务配置不完整")
	}
	return &Service{files: map[string]*SessionFile{}, frames: map[string]*imageEntry{}, tokens: map[string]*sourceToken{}, sem: make(chan struct{}, 1), opts: opts}, nil
}

func (s *Service) Add(ctx context.Context, accountID int64, fileID, parentID string, directoryChain []DirectoryRef) (*SessionFile, error) {
	item, err := s.opts.Files.Info(ctx, accountID, fileID)
	if err != nil {
		return nil, err
	}
	if item.IsDir {
		return nil, domain.Errorf(domain.CodeValidation, "目录不能提取封面")
	}
	if !IsSupported(item.Name) {
		return nil, domain.Errorf(domain.CodeValidation, "仅支持 MP4、MKV、MOV 和 WebM 视频")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	for _, existing := range s.files {
		if existing.AccountID == accountID && existing.FileID == fileID {
			existing.TouchedAt = time.Now().Unix()
			return cloneFile(existing), nil
		}
	}
	if len(s.files) >= maxFiles {
		return nil, domain.Errorf(domain.CodeValidation, "视频海报生成列表最多保留 %d 个视频", maxFiles)
	}
	targetParentID, targetPath := defaultTarget(parentID, directoryChain)
	f := &SessionFile{ID: uuid.NewString(), AccountID: accountID, FileID: fileID, ParentID: parentID, TargetParentID: targetParentID, TargetPath: targetPath, Name: item.Name, Size: item.Size, Status: "queued", Frames: []Frame{}, TouchedAt: time.Now().Unix()}
	s.files[f.ID] = f
	return cloneFile(f), nil
}

func defaultTarget(parentID string, chain []DirectoryRef) (string, string) {
	usable := make([]DirectoryRef, 0, len(chain))
	rootID := ""
	for _, dir := range chain {
		name := strings.TrimSpace(dir.Name)
		if name == "根目录" {
			rootID = dir.ID
			continue
		}
		if name == "" {
			continue
		}
		usable = append(usable, DirectoryRef{ID: dir.ID, Name: name})
	}
	if len(usable) == 0 {
		return parentID, "/"
	}
	target := len(usable) - 1
	if rules.IsSeasonDirName(usable[target].Name) {
		target--
	}
	if target < 0 {
		return rootID, "/"
	}
	names := make([]string, 0, target+1)
	for _, dir := range usable[:target+1] {
		names = append(names, dir.Name)
	}
	return usable[target].ID, "/" + strings.Join(names, "/")
}

func (s *Service) SetTarget(id, parentID, path string) (*SessionFile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.files[id]
	if f == nil {
		return nil, domain.Errorf(domain.CodeNotFound, "视频不在视频海报生成列表中")
	}
	f.TargetParentID = parentID
	f.TargetPath = path
	f.TouchedAt = time.Now().Unix()
	return cloneFile(f), nil
}

func IsSupported(name string) bool {
	_, ok := supportedExt[strings.ToLower(filepath.Ext(name))]
	return ok
}

func (s *Service) List() []*SessionFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	out := make([]*SessionFile, 0, len(s.files))
	for _, f := range s.files {
		out = append(out, cloneFile(f))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TouchedAt > out[j].TouchedAt })
	return out
}

func (s *Service) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f := s.files[id]; f != nil {
		for _, frame := range f.Frames {
			s.removeFrameLocked(frame.ID)
		}
		delete(s.files, id)
	}
}
func (s *Service) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files = map[string]*SessionFile{}
	s.frames = map[string]*imageEntry{}
	s.tokens = map[string]*sourceToken{}
	s.imageLen = 0
}

func (s *Service) Runtime() map[string]any {
	ffmpeg, ferr := findTool(s.opts.DataDir, "ffmpeg")
	return map[string]any{"ready": ferr == nil, "ffmpeg": ffmpeg, "error": joinToolErrors(ferr), "auto_download_available": supportedDownloadAsset() != nil, "manual_path": filepath.Join(s.opts.DataDir, "tools")}
}

func (s *Service) Extract(ctx context.Context, req ExtractRequest) (*SessionFile, error) {
	s.mu.Lock()
	f := s.files[req.SessionFileID]
	if f == nil {
		s.mu.Unlock()
		return nil, domain.Errorf(domain.CodeNotFound, "视频不在视频海报生成列表中")
	}
	f.Status = "extracting"
	f.Error = ""
	local := *f
	s.mu.Unlock()
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	ffmpeg, err := findTool(s.opts.DataDir, "ffmpeg")
	if err != nil {
		return s.fail(req.SessionFileID, err)
	}
	token, sourceURL, err := s.newSource(local.AccountID, local.FileID)
	if err != nil {
		return s.fail(req.SessionFileID, err)
	}
	defer s.dropToken(token)
	duration, err := probeDuration(ctx, ffmpeg, sourceURL)
	if err != nil {
		return s.fail(req.SessionFileID, err)
	}
	times, err := extractionTimes(req, duration)
	if err != nil {
		return s.fail(req.SessionFileID, err)
	}
	for _, ts := range times {
		data, extractErr := extractOne(ctx, ffmpeg, sourceURL, ts)
		if extractErr != nil {
			continue
		}
		s.addFrame(req.SessionFileID, ts, data)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f = s.files[req.SessionFileID]
	if f == nil {
		return nil, domain.Errorf(domain.CodeNotFound, "会话已清理")
	}
	f.DurationMS = duration
	f.TouchedAt = time.Now().Unix()
	if len(f.Frames) == 0 {
		f.Status = "failed"
		f.Error = "未能提取可用画面"
	} else {
		f.Status = "done"
	}
	return cloneFile(f), nil
}

func (s *Service) Image(id string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	v, ok := s.frames[id]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), v.Data...), true
}

func (s *Service) Save(ctx context.Context, req SaveRequest) (SaveResult, error) {
	s.mu.Lock()
	f := s.files[req.SessionFileID]
	img := s.frames[req.FrameID]
	if f == nil || img == nil || !frameBelongsToFile(f, req.FrameID) {
		s.mu.Unlock()
		return SaveResult{}, domain.Errorf(domain.CodeNotFound, "文件或候选图已失效")
	}
	local, data := *f, append([]byte(nil), img.Data...)
	s.mu.Unlock()
	return s.saveData(ctx, local, data, req.Overwrite)
}

// SaveComposed 保存浏览器 Canvas 生成的最终海报。候选帧仍用于校验会话归属，
// 避免客户端绕过取帧流程向任意网盘目录写入图片。
func (s *Service) SaveComposed(ctx context.Context, req SaveRequest, data []byte) (SaveResult, error) {
	if len(data) == 0 || int64(len(data)) > MaxPosterBytes {
		return SaveResult{}, domain.Errorf(domain.CodeValidation, "合成海报大小无效")
	}
	s.mu.Lock()
	f := s.files[req.SessionFileID]
	img := s.frames[req.FrameID]
	if f == nil || img == nil || !frameBelongsToFile(f, req.FrameID) {
		s.mu.Unlock()
		return SaveResult{}, domain.Errorf(domain.CodeNotFound, "文件或候选图已失效")
	}
	local := *f
	s.mu.Unlock()
	return s.saveData(ctx, local, append([]byte(nil), data...), req.Overwrite)
}

func frameBelongsToFile(file *SessionFile, frameID string) bool {
	if file == nil || frameID == "" {
		return false
	}
	for _, frame := range file.Frames {
		if frame.ID == frameID {
			return true
		}
	}
	return false
}

func (s *Service) saveData(ctx context.Context, local SessionFile, data []byte, overwrite bool) (SaveResult, error) {
	items, err := s.opts.Files.List(ctx, local.AccountID, local.TargetParentID, true)
	if err != nil {
		return SaveResult{}, err
	}
	filename := "poster.jpg"
	for _, item := range items {
		if strings.EqualFold(item.Name, filename) && !overwrite {
			return SaveResult{Conflict: true, Filename: filename}, nil
		}
	}
	dir := filepath.Join(s.opts.DataDir, "coverextract")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return SaveResult{}, err
	}
	tmp, err := os.CreateTemp(dir, "cover-*.jpg")
	if err != nil {
		return SaveResult{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return SaveResult{}, err
	}
	policy := "fail"
	if overwrite {
		policy = "overwrite"
	}
	_, err = s.opts.Files.UploadLocal(ctx, local.AccountID, driver.LocalUploadRequest{LocalPath: tmpPath, FileName: filename, ParentID: local.TargetParentID, ConflictPolicy: policy})
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{OK: true, Filename: filename}, nil
}

func (s *Service) ServeSource(w http.ResponseWriter, r *http.Request, token string) error {
	if !isLoopback(r.RemoteAddr) {
		return domain.Errorf(domain.CodePermissionDenied, "临时媒体入口仅限本机")
	}
	s.mu.Lock()
	st := s.tokens[token]
	if st == nil || time.Now().After(st.ExpiresAt) {
		delete(s.tokens, token)
		s.mu.Unlock()
		return domain.Errorf(domain.CodePermissionDenied, "临时媒体凭证已失效")
	}
	accountID, fileID := st.AccountID, st.FileID
	s.mu.Unlock()
	cw := &countingWriter{ResponseWriter: w, add: func(n int64) bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		cur := s.tokens[token]
		if cur == nil {
			return false
		}
		cur.Read += n
		return cur.Read <= cur.MaxRead
	}}
	// 必须由 LitePan 代理字节，否则 302 后 FFmpeg 会绕过读取预算。
	return s.opts.Playback.ServeHTTP(cw, r, playback.Request{AccountID: accountID, FileID: fileID}, playback.Intent{FileName: "source", ForceProxy: true})
}

type countingWriter struct {
	http.ResponseWriter
	add func(int64) bool
}

func (w *countingWriter) Write(p []byte) (int, error) {
	if !w.add(int64(len(p))) {
		return 0, domain.Errorf(domain.CodeValidation, "取帧读取量超过安全上限")
	}
	return w.ResponseWriter.Write(p)
}

func (s *Service) newSource(accountID int64, fileID string) (string, string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(b)
	s.mu.Lock()
	s.tokens[token] = &sourceToken{AccountID: accountID, FileID: fileID, ExpiresAt: time.Now().Add(10 * time.Minute), MaxRead: defaultReadMax}
	s.mu.Unlock()
	return token, "http://127.0.0.1" + normalizeListen(s.opts.ListenAddr) + "/api/internal/cover-source/" + token, nil
}
func normalizeListen(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ":5211"
	}
	if strings.HasPrefix(addr, ":") {
		return addr
	}
	if _, p, ok := strings.Cut(addr, ":"); ok {
		return ":" + p
	}
	return ":5211"
}

func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}
func (s *Service) dropToken(token string) { s.mu.Lock(); delete(s.tokens, token); s.mu.Unlock() }

func extractionTimes(req ExtractRequest, duration int64) ([]int64, error) {
	if duration <= 0 {
		return nil, domain.Errorf(domain.CodeValidation, "无法读取视频时长")
	}
	switch req.Mode {
	case "timestamp":
		if req.TimestampMS < 0 || req.TimestampMS >= duration {
			return nil, domain.Errorf(domain.CodeValidation, "时间点必须在视频时长内")
		}
		return []int64{req.TimestampMS}, nil
	case "head_tail", "head":
		if duration <= 1000 {
			return []int64{0}, nil
		}
		return []int64{500, duration - 500}, nil
	case "uniform", "":
		n := req.Count
		if n == 0 {
			n = 5
		}
		if n < 3 || n > 9 {
			return nil, domain.Errorf(domain.CodeValidation, "均匀取帧数必须在 3 到 9 之间")
		}
		out := make([]int64, 0, n)
		for i := 1; i <= n; i++ {
			out = append(out, duration*int64(i)/int64(n+1))
		}
		return out, nil
	default:
		return nil, domain.Errorf(domain.CodeValidation, "不支持的取帧方式")
	}
}

var durationPattern = regexp.MustCompile(`Duration:\s*(\d+):(\d+):(\d+(?:\.\d+)?)`)

func probeDuration(parent context.Context, bin, url string) (int64, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, bin, "-hide_banner", "-i", url).CombinedOutput()
	match := durationPattern.FindSubmatch(out)
	if len(match) != 4 {
		return 0, errors.New("FFmpeg 未返回有效时长")
	}
	hours, _ := strconv.ParseInt(string(match[1]), 10, 64)
	minutes, _ := strconv.ParseInt(string(match[2]), 10, 64)
	seconds, err := strconv.ParseFloat(string(match[3]), 64)
	if err != nil {
		return 0, errors.New("FFmpeg 时长格式无效")
	}
	return int64((float64(hours*3600+minutes*60) + seconds) * 1000), nil
}
func extractOne(parent context.Context, bin, url string, ts int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "litepan-cover-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "frame.jpg")
	stamp := fmt.Sprintf("%.3f", float64(ts)/1000)
	cmd := exec.CommandContext(ctx, bin, "-hide_banner", "-loglevel", "error", "-y", "-ss", stamp, "-i", url, "-frames:v", "1", "-vf", "scale=w='min(1280,iw)':h='min(1280,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2", "-q:v", "2", out)
	if b, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("FFmpeg 取帧失败: %s", strings.TrimSpace(string(b)))
	}
	return os.ReadFile(out)
}

func findTool(dataDir, name string) (string, error) {
	local := filepath.Join(dataDir, "tools", name)
	if st, err := os.Stat(local); err == nil && !st.IsDir() {
		return local, nil
	}
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("未找到 %s，请放入 %s", name, local)
	}
	return p, nil
}
func joinToolErrors(errs ...error) string {
	var parts []string
	for _, err := range errs {
		if err != nil {
			parts = append(parts, err.Error())
		}
	}
	return strings.Join(parts, "；")
}

type downloadAsset struct{ Name, SHA256 string }

func supportedDownloadAsset() *downloadAsset {
	assets := map[string]downloadAsset{
		"linux/amd64":  {Name: "ffmpeg-linux-x64.gz", SHA256: "bfe8a8fc511530457b528c48d77b5737527b504a3797a9bc4866aeca69c2dffa"},
		"linux/arm64":  {Name: "ffmpeg-linux-arm64.gz", SHA256: "754a678672298bc68156adff58aa7385a592c2b30b1d0ae8750c45c915c4bac0"},
		"darwin/amd64": {Name: "ffmpeg-darwin-x64.gz", SHA256: "929b375c1182d956c51f7ac25e0b2b0411fb01f6f407aa15c9758efeb4242106"},
		"darwin/arm64": {Name: "ffmpeg-darwin-arm64.gz", SHA256: "8923876afa8db5585022d7860ec7e589af192f441c56793971276d450ed3bbfa"},
	}
	v, ok := assets[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		return nil
	}
	return &v
}

func (s *Service) DownloadFFmpeg(ctx context.Context) error {
	s.downloadMu.Lock()
	defer s.downloadMu.Unlock()
	asset := supportedDownloadAsset()
	if asset == nil {
		return domain.Errorf(domain.CodeNotImplement, "当前平台不支持自动安装 FFmpeg")
	}
	if _, err := findTool(s.opts.DataDir, "ffmpeg"); err == nil {
		return nil
	}
	urls := []string{
		"https://gitcode.com/gh_mirrors/ff/ffmpeg-static/releases/download/b6.1.1/" + asset.Name,
		"https://github.com/eugeneware/ffmpeg-static/releases/download/b6.1.1/" + asset.Name,
	}
	var last error
	for _, url := range urls {
		if err := s.downloadOne(ctx, url, *asset); err == nil {
			return nil
		} else {
			last = err
		}
	}
	return fmt.Errorf("FFmpeg 下载失败: %w", last)
}

func (s *Service) downloadOne(ctx context.Context, url string, asset downloadAsset) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载源返回 HTTP %d", resp.StatusCode)
	}
	dir := filepath.Join(s.opts.DataDir, "tools")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	archive, err := os.CreateTemp(dir, "ffmpeg-*.gz")
	if err != nil {
		return err
	}
	archivePath := archive.Name()
	defer os.Remove(archivePath)
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(archive, h), io.LimitReader(resp.Body, 128<<20))
	closeErr := archive.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if n < 10<<20 {
		return errors.New("下载内容过小，可能不是 FFmpeg 资产")
	}
	if hex.EncodeToString(h.Sum(nil)) != asset.SHA256 {
		return errors.New("FFmpeg SHA-256 校验失败")
	}
	in, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer in.Close()
	gz, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer gz.Close()
	tmp, err := os.CreateTemp(dir, "ffmpeg-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err = io.Copy(tmp, io.LimitReader(gz, 256<<20)); err == nil {
		err = tmp.Sync()
	}
	closeErr = tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Chmod(tmpPath, 0o755); err != nil {
		return err
	}
	smokeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, smokeErr := exec.CommandContext(smokeCtx, tmpPath, "-version").CombinedOutput()
	if smokeErr != nil || !strings.Contains(string(out), "ffmpeg version") {
		return errors.New("FFmpeg 冒烟检查失败")
	}
	return os.Rename(tmpPath, filepath.Join(dir, "ffmpeg"))
}
func (s *Service) fail(id string, err error) (*SessionFile, error) {
	s.mu.Lock()
	if f := s.files[id]; f != nil {
		f.Status = "failed"
		f.Error = err.Error()
		f.TouchedAt = time.Now().Unix()
	}
	s.mu.Unlock()
	return nil, err
}
func (s *Service) addFrame(fileID string, ts int64, data []byte) {
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.files[fileID]
	if f == nil {
		return
	}
	for _, fr := range f.Frames {
		if img := s.frames[fr.ID]; img != nil {
			old := sha256.Sum256(img.Data)
			if hex.EncodeToString(old[:]) == digest {
				return
			}
		}
	}
	id := uuid.NewString()
	s.frames[id] = &imageEntry{Data: append([]byte(nil), data...), CreatedAt: time.Now()}
	s.imageLen += int64(len(data))
	f.Frames = append(f.Frames, Frame{ID: id, TimeMS: ts})
	s.trimImagesLocked()
}
func (s *Service) cleanupLocked() {
	cut := time.Now().Add(-2 * time.Hour).Unix()
	for id, f := range s.files {
		if f.TouchedAt < cut {
			for _, fr := range f.Frames {
				s.removeFrameLocked(fr.ID)
			}
			delete(s.files, id)
		}
	}
	for id, t := range s.tokens {
		if time.Now().After(t.ExpiresAt) {
			delete(s.tokens, id)
		}
	}
}
func (s *Service) trimImagesLocked() {
	for len(s.frames) > maxFrames || s.imageLen > maxImageBytes {
		var oldest string
		var at time.Time
		for id, v := range s.frames {
			if oldest == "" || v.CreatedAt.Before(at) {
				oldest = id
				at = v.CreatedAt
			}
		}
		if oldest == "" {
			break
		}
		s.removeFrameLocked(oldest)
		for _, f := range s.files {
			next := f.Frames[:0]
			for _, fr := range f.Frames {
				if fr.ID != oldest {
					next = append(next, fr)
				}
			}
			f.Frames = next
		}
	}
}
func (s *Service) removeFrameLocked(id string) {
	if v := s.frames[id]; v != nil {
		s.imageLen -= int64(len(v.Data))
		delete(s.frames, id)
	}
}
func cloneFile(f *SessionFile) *SessionFile {
	if f == nil {
		return nil
	}
	v := *f
	// API 对空候选集始终返回 []，避免前端读取 length 时遇到 null。
	v.Frames = append([]Frame{}, f.Frames...)
	return &v
}
