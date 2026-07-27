package crosstransfer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"litepan/internal/core/driverexec"
	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/file"
	"litepan/internal/playback"
)

type Service struct {
	exec  *driverexec.Executor
	files *file.Service
	relay *RelayManager
	log   *slog.Logger
}

type Options struct {
	Exec     *driverexec.Executor
	Files    *file.Service
	Playback *playback.Service
	DataDir  string
	Log      *slog.Logger
}

func New(opts Options) *Service {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	relay := NewRelayManager(RelayOptions{
		Exec:     opts.Exec,
		Files:    opts.Files,
		Playback: opts.Playback,
		DataDir:  opts.DataDir,
		Log:      log,
	})
	return &Service{exec: opts.Exec, files: opts.Files, relay: relay, log: log}
}

func (s *Service) Relay() *RelayManager { return s.relay }

type ScanFile struct {
	SourceFileID string `json:"source_file_id"`
	RelPath      string `json:"rel_path"`
	RelDir       string `json:"rel_dir"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	Hash         string `json:"hash"`
	Eligible     bool   `json:"eligible"`
}

type ScanTreeNode struct {
	Type     string         `json:"type"`
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	RelPath  string         `json:"rel_path,omitempty"`
	RelDir   string         `json:"rel_dir,omitempty"`
	Size     int64          `json:"size,omitempty"`
	Hash     string         `json:"hash,omitempty"`
	Eligible bool           `json:"eligible,omitempty"`
	Children []ScanTreeNode `json:"children,omitempty"`
}

type ScanResult struct {
	Tree        []ScanTreeNode `json:"tree"`
	Total       int            `json:"total"`
	ShallowDirs int            `json:"shallow_dirs"`
	Truncated   bool           `json:"truncated"`
	Files       []ScanFile     `json:"files"`
}

type TransferFile struct {
	SourceFileID string `json:"source_file_id"`
	RelPath      string `json:"rel_path"`
	RelDir       string `json:"rel_dir"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	Hash         string `json:"hash"`
}

type ExecuteInput struct {
	SourceAccountID   int64
	SourceAccountName string
	SourceDriverType  string
	TargetAccountID   int64
	TargetAccountName string
	TargetDriverType  string
	TargetParentID    string
	TargetDisplayPath string
	MethodID          string
	Files             []TransferFile
	Conflict          string
	Fallback          bool
}

func sourceRootPrefix(displayPath string) string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(displayPath), "/"), "/")
	var cleaned []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			cleaned = append(cleaned, p)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	return cleaned[len(cleaned)-1] + "/"
}

func (s *Service) ScanSource(ctx context.Context, sourceAccountID int64, sourceParentID, methodID, sourceDisplayPath string) (*ScanResult, error) {
	if _, ok := GetMethod(methodID); !ok {
		return nil, domain.Errorf(domain.CodeValidation, "未知的秒传方法: %s", methodID)
	}
	rootPrefix := sourceRootPrefix(sourceDisplayPath)
	acc := &scanAccumulator{files: make([]scanFileRec, 0, 256)}
	sem := make(chan struct{}, scanDirConcurrency)
	var accMu sync.Mutex

	tree, err := s.walkSource(ctx, sourceAccountID, sourceParentID, methodID, rootPrefix, 0, acc, &accMu, sem)
	if err != nil {
		return nil, err
	}

	outFiles := make([]ScanFile, 0, len(acc.files))
	for _, f := range acc.files {
		outFiles = append(outFiles, ScanFile{
			SourceFileID: f.id,
			RelPath:      f.relPath,
			RelDir:       f.relDir,
			Name:         f.name,
			Size:         f.size,
			Hash:         f.hash,
			Eligible:     f.hash != "",
		})
	}
	outFiles = orderScanFilesByTree(tree, outFiles)
	return &ScanResult{
		Tree:        tree,
		Total:       len(outFiles),
		ShallowDirs: countShallowDirs(tree),
		Truncated:   acc.count >= maxScanFiles,
		Files:       outFiles,
	}, nil
}

type scanFileRec struct {
	id, relPath, relDir, name, hash string
	size                            int64
}

type scanAccumulator struct {
	mu    sync.Mutex
	count int
	files []scanFileRec
}

func (a *scanAccumulator) appendFile(rec scanFileRec) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.count >= maxScanFiles {
		return false
	}
	a.files = append(a.files, rec)
	a.count++
	return true
}

func (a *scanAccumulator) atLimit() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.count >= maxScanFiles
}

func (s *Service) walkSource(
	ctx context.Context,
	accountID int64,
	parentID, methodID, relPrefix string,
	depth int,
	acc *scanAccumulator,
	accMu *sync.Mutex,
	sem chan struct{},
) ([]ScanTreeNode, error) {
	if depth > maxScanDepth || acc.atLimit() {
		return nil, nil
	}
	items, err := s.files.List(ctx, accountID, parentID, false)
	if err != nil {
		return nil, err
	}

	var dirs []domain.FileItem
	var files []domain.FileItem
	for _, it := range items {
		if it.IsDir {
			dirs = append(dirs, it)
		} else {
			files = append(files, it)
		}
	}

	var wg sync.WaitGroup
	dirNodes := make([]ScanTreeNode, len(dirs))
	for i, dir := range dirs {
		if acc.atLimit() {
			break
		}
		wg.Add(1)
		go func(i int, dir domain.FileItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			childPrefix := relPrefix + dir.Name + "/"
			children, err := s.walkSource(ctx, accountID, dir.ID, methodID, childPrefix, depth+1, acc, accMu, sem)
			if err != nil || acc.atLimit() {
				return
			}
			dirNodes[i] = ScanTreeNode{Type: "dir", ID: dir.ID, Name: dir.Name, Children: children}
		}(i, dir)
	}
	wg.Wait()

	var out []ScanTreeNode
	for _, node := range dirNodes {
		if node.ID != "" {
			out = append(out, node)
		}
	}

	for _, it := range files {
		if acc.atLimit() {
			break
		}
		hash, err := s.resolveHash(ctx, accountID, &it, methodID, false)
		if err != nil {
			return nil, err
		}
		rec := scanFileRec{
			id:      it.ID,
			name:    it.Name,
			size:    it.Size,
			hash:    hash,
			relPath: relPrefix + it.Name,
			relDir:  strings.TrimSuffix(relPrefix, "/"),
		}
		if !acc.appendFile(rec) {
			break
		}
		out = append(out, ScanTreeNode{
			Type:     "file",
			ID:       rec.id,
			Name:     rec.name,
			RelPath:  rec.relPath,
			RelDir:   rec.relDir,
			Size:     rec.size,
			Hash:     rec.hash,
			Eligible: rec.hash != "",
		})
	}
	return out, nil
}

func flattenTreeFilePaths(nodes []ScanTreeNode) []string {
	out := make([]string, 0, 64)
	var walk func([]ScanTreeNode)
	walk = func(list []ScanTreeNode) {
		for _, n := range list {
			if n.Type == "dir" {
				walk(n.Children)
				continue
			}
			if n.RelPath != "" {
				out = append(out, n.RelPath)
			}
		}
	}
	walk(nodes)
	return out
}

func orderScanFilesByTree(tree []ScanTreeNode, files []ScanFile) []ScanFile {
	if len(files) == 0 || len(tree) == 0 {
		return files
	}
	byPath := make(map[string]ScanFile, len(files))
	for _, f := range files {
		byPath[f.RelPath] = f
	}
	ordered := make([]ScanFile, 0, len(files))
	seen := make(map[string]struct{}, len(files))
	for _, relPath := range flattenTreeFilePaths(tree) {
		f, ok := byPath[relPath]
		if !ok {
			continue
		}
		ordered = append(ordered, f)
		seen[relPath] = struct{}{}
	}
	for _, f := range files {
		if _, ok := seen[f.RelPath]; ok {
			continue
		}
		ordered = append(ordered, f)
	}
	return ordered
}

func countShallowDirs(tree []ScanTreeNode) int {
	total := 0
	for _, node := range tree {
		if node.Type != "dir" {
			continue
		}
		total++
		for _, child := range node.Children {
			if child.Type == "dir" {
				total++
			}
		}
	}
	return total
}

func (s *Service) resolveHash(ctx context.Context, accountID int64, item *domain.FileItem, methodID string, allowStream bool) (string, error) {
	if h := driver.HashFromItem(item, methodID); h != "" {
		return h, nil
	}
	if !allowStream || item == nil || strings.TrimSpace(item.ID) == "" {
		return "", nil
	}
	var hash string
	err := s.exec.Run(ctx, accountID, func(drv driver.Driver) error {
		if resolver, ok := drv.(driver.TransferHashResolver); ok {
			got, err := resolver.ResolveTransferHash(ctx, item, methodID, true)
			if err != nil {
				return err
			}
			hash = got
			return nil
		}
		if info, ok := drv.(driver.InfoGetter); ok {
			got, err := info.GetFileInfo(ctx, item.ID)
			if err != nil {
				return err
			}
			hash = driver.HashFromItem(got, methodID)
		}
		return nil
	})
	return hash, err
}

func (s *Service) ensureFileHash(ctx context.Context, sourceAccountID int64, f *TransferFile, methodID string, allowStream bool) (string, error) {
	if h := strings.TrimSpace(f.Hash); h != "" {
		return strings.ToLower(h), nil
	}
	sourceFileID := strings.TrimSpace(f.SourceFileID)
	if sourceFileID == "" {
		return "", nil
	}
	item, err := s.files.Info(ctx, sourceAccountID, sourceFileID)
	if err != nil {
		return "", err
	}
	hash, err := s.resolveHash(ctx, sourceAccountID, item, methodID, allowStream)
	if err != nil {
		return "", err
	}
	if hash != "" {
		f.Hash = hash
	}
	return hash, nil
}

type StreamEvent map[string]any

func (s *Service) ProbeStream(ctx context.Context, sourceAccountID, targetAccountID int64, targetParentID, methodID string, files []TransferFile, emit func(StreamEvent) error) error {
	if _, ok := GetMethod(methodID); !ok {
		return domain.Errorf(domain.CodeValidation, "未知的秒传方法: %s", methodID)
	}
	if err := emit(StreamEvent{"event": "start", "total": len(files)}); err != nil {
		return err
	}

	probeFolderID := ""
	okCount := 0
	noCount := 0
	directProbe, err := s.supportsRapidProbe(ctx, targetAccountID, methodID)
	if err != nil {
		return err
	}
	defer func() {
		if probeFolderID != "" {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			if err := s.files.DeleteFiles(cleanupCtx, targetAccountID, []string{probeFolderID}, targetParentID); err != nil {
				s.log.Warn("清理秒传探测目录失败", "folder_id", probeFolderID, "err", err)
			}
		}
	}()

	probeParentID := targetParentID
	if !directProbe {
		probeName := fmt.Sprintf("_litepan_probe_%d", time.Now().Unix())
		created, err := s.files.CreateFolder(ctx, targetAccountID, targetParentID, probeName)
		if err != nil {
			return emit(StreamEvent{"event": "error", "message": "创建临时探测目录失败: " + err.Error()})
		}
		probeFolderID = created.ID
		probeParentID = probeFolderID
	}

	for i := range files {
		f := &files[i]
		fileHash := strings.TrimSpace(f.Hash)
		if fileHash == "" {
			_ = emit(StreamEvent{"event": "hashing", "rel_path": f.RelPath, "name": f.Name})
			var err error
			fileHash, err = s.ensureFileHash(ctx, sourceAccountID, f, methodID, true)
			if err != nil {
				s.log.Warn("跨盘秒传计算指纹失败", "name", f.Name, "err", err)
			}
		}
		reuse := false
		probeErr := ""
		var terminalErr error
		if fileHash != "" {
			if directProbe {
				var err error
				reuse, err = s.tryRapidProbe(ctx, targetAccountID, probeParentID, f.Name, methodID, fileHash, f.Size)
				if err != nil {
					probeErr = err.Error()
					if driver.IsRapidProbeTerminal(err) {
						terminalErr = err
					}
				}
			} else {
				var errMsg string
				reuse, _, errMsg = s.tryRapidUpload(ctx, targetAccountID, probeParentID, f.Name, methodID, fileHash, f.Size, 2)
				probeErr = errMsg
			}
			if probeErr != "" {
				s.log.Warn("跨盘秒传试探失败", "name", f.Name, "err", probeErr)
			}
		}
		if reuse {
			okCount++
		} else {
			noCount++
		}
		if err := emit(StreamEvent{
			"event":    "item",
			"rel_path": f.RelPath,
			"reuse":    reuse,
			"hash":     fileHash,
			"error":    probeErr,
		}); err != nil {
			return err
		}
		if terminalErr != nil {
			return terminalErr
		}
	}
	return emit(StreamEvent{"event": "end", "ok": okCount, "no": noCount})
}

func (s *Service) ExecuteStream(ctx context.Context, in ExecuteInput, emit func(StreamEvent) error) error {
	if _, ok := GetMethod(in.MethodID); !ok {
		return domain.Errorf(domain.CodeValidation, "未知的秒传方法: %s", in.MethodID)
	}
	duplicate := 1
	conflict := normalizeConflictPolicy(in.Conflict)
	if conflict == "overwrite" {
		duplicate = 2
	}
	dirCache := map[string]string{"": in.TargetParentID}
	var dirCreated []createdTargetDir
	keptDirs := map[string]struct{}{}
	var results []map[string]any
	relayQueued := 0
	cleanupDone := in.Fallback
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		s.cleanupCreatedDirs(cleanupCtx, in.TargetAccountID, dirCreated, keptDirs)
		cleanupDone = true
	}
	defer func() {
		if !cleanupDone {
			cleanup()
		}
	}()

	if err := emit(StreamEvent{"event": "start", "total": len(in.Files)}); err != nil {
		return err
	}

	for i := range in.Files {
		item := s.executeTransferFile(ctx, executeFileInput{
			file:              &in.Files[i],
			methodID:          in.MethodID,
			targetAccountID:   in.TargetAccountID,
			targetParentID:    in.TargetParentID,
			dirCache:          dirCache,
			dirCreated:        &dirCreated,
			duplicate:         duplicate,
			fallback:          in.Fallback,
			sourceAccountID:   in.SourceAccountID,
			sourceAccountName: in.SourceAccountName,
			sourceDriverType:  in.SourceDriverType,
			targetAccountName: in.TargetAccountName,
			targetDriverType:  in.TargetDriverType,
			targetDisplayPath: in.TargetDisplayPath,
			conflict:          conflict,
		})
		results = append(results, item)
		if item["mode"] == "relay" {
			relayQueued++
		}
		if item["success"] == true {
			markKeptDir(keptDirs, in.Files[i].RelDir)
		}
		if err := emit(item); err != nil {
			return err
		}
	}

	if !in.Fallback {
		cleanup()
	}

	rapidDone := 0
	for _, r := range results {
		if r["mode"] == "rapid" && r["success"] == true {
			rapidDone++
		}
	}
	return emit(StreamEvent{
		"event":        "end",
		"done":         rapidDone,
		"total":        len(in.Files),
		"rapid_done":   rapidDone,
		"relay_queued": relayQueued,
		"results":      results,
	})
}

type executeFileInput struct {
	file              *TransferFile
	methodID          string
	targetAccountID   int64
	targetParentID    string
	dirCache          map[string]string
	dirCreated        *[]createdTargetDir
	duplicate         int
	fallback          bool
	sourceAccountID   int64
	sourceAccountName string
	sourceDriverType  string
	targetAccountName string
	targetDriverType  string
	targetDisplayPath string
	conflict          string
}

func (s *Service) executeTransferFile(ctx context.Context, in executeFileInput) map[string]any {
	f := in.file
	base := map[string]any{
		"event":    "item",
		"rel_path": f.RelPath,
		"name":     f.Name,
	}
	folderID, err := EnsureTargetDir(ctx, s.files, in.targetAccountID, in.targetParentID, f.RelDir, in.dirCache, in.dirCreated)
	if err != nil {
		s.log.Warn("跨盘秒传创建目录失败", "name", f.Name, "err", err)
		return transferItemResult(base, false, "error", "", err.Error())
	}
	if in.conflict == "skip" {
		exists, err := s.targetFileExists(ctx, in.targetAccountID, folderID, f.Name)
		if err != nil {
			s.log.Warn("跨盘秒传检查目标同名失败", "name", f.Name, "err", err)
			return transferItemResult(base, false, "error", "", err.Error())
		}
		if exists {
			return transferItemResult(base, true, "skip", "", "")
		}
	}

	fileHash, err := s.ensureFileHash(ctx, in.sourceAccountID, f, in.methodID, true)
	if err != nil {
		s.log.Warn("跨盘秒传执行前取指纹失败", "name", f.Name, "err", err)
	}
	if fileHash == "" {
		return transferItemResult(base, false, "skip", "", "缺少指纹")
	}

	reuse, fileID, rapidErr := s.tryRapidUpload(ctx, in.targetAccountID, folderID, f.Name, in.methodID, fileHash, f.Size, in.duplicate)
	if rapidErr != "" {
		return transferItemResult(base, false, "error", "", rapidErr)
	}
	if reuse {
		return transferItemResult(base, true, "rapid", fileID, "")
	}

	if in.fallback && strings.TrimSpace(f.SourceFileID) != "" {
		_, err := s.relay.CreateTask(ctx, RelayTaskInput{
			SourceAccountID:   in.sourceAccountID,
			SourceAccountName: in.sourceAccountName,
			SourceDriverType:  in.sourceDriverType,
			TargetAccountID:   in.targetAccountID,
			TargetAccountName: in.targetAccountName,
			TargetDriverType:  in.targetDriverType,
			SourceFileID:      f.SourceFileID,
			FileName:          f.Name,
			RelPath:           f.RelPath,
			RelDir:            f.RelDir,
			TargetParentID:    in.targetParentID,
			TargetDisplayPath: in.targetDisplayPath,
			TotalBytes:        f.Size,
			Method:            in.methodID,
			ConflictPolicy:    in.conflict,
		})
		if err != nil {
			return transferItemResult(base, false, "error", "", err.Error())
		}
		return transferItemResult(base, false, "relay", "", "")
	}

	return transferItemResult(base, false, "rapid", "", "未命中秒传")
}

func normalizeConflictPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "skip", "rename", "overwrite":
		return strings.ToLower(strings.TrimSpace(policy))
	default:
		return "skip"
	}
}

func (s *Service) targetFileExists(ctx context.Context, accountID int64, parentID, name string) (bool, error) {
	items, err := s.files.List(ctx, accountID, parentID, false)
	if err != nil {
		return false, err
	}
	for _, item := range items {
		if item.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func transferItemResult(base map[string]any, success bool, mode, fileID, errMsg string) map[string]any {
	base["success"] = success
	base["mode"] = mode
	base["file_id"] = fileID
	base["error"] = errMsg
	return base
}

func (s *Service) tryRapidUpload(ctx context.Context, targetAccountID int64, folderID, name, methodID, hash string, size int64, duplicate int) (reuse bool, fileID string, errMsg string) {
	err := s.exec.Run(ctx, targetAccountID, func(drv driver.Driver) error {
		uploader, err := driverexec.Require[driver.RapidUploader](drv)
		if err != nil {
			return err
		}
		result, err := uploader.RapidUploadByHash(ctx, driver.RapidUploadRequest{
			ParentID:  folderID,
			FileName:  name,
			Method:    methodID,
			Hash:      hash,
			Size:      size,
			Duplicate: duplicate,
		})
		if err != nil {
			return err
		}
		reuse = result.Reuse
		fileID = result.FileID
		return nil
	})
	if err != nil {
		return false, "", err.Error()
	}
	return reuse, fileID, ""
}

func (s *Service) supportsRapidProbe(ctx context.Context, targetAccountID int64, methodID string) (bool, error) {
	supported := false
	err := s.exec.Run(ctx, targetAccountID, func(drv driver.Driver) error {
		prober, ok := drv.(driver.RapidUploadProber)
		supported = ok && prober.SupportsRapidUploadProbe(methodID)
		return nil
	})
	return supported, err
}

func (s *Service) tryRapidProbe(ctx context.Context, targetAccountID int64, parentID, name, methodID, hash string, size int64) (bool, error) {
	reuse := false
	err := s.exec.Run(ctx, targetAccountID, func(drv driver.Driver) error {
		prober, err := driverexec.Require[driver.RapidUploadProber](drv)
		if err != nil {
			return err
		}
		result, err := prober.ProbeRapidUploadByHash(ctx, driver.RapidUploadRequest{
			ParentID: parentID,
			FileName: name,
			Method:   methodID,
			Hash:     hash,
			Size:     size,
		})
		if err != nil {
			return err
		}
		if result == nil {
			return domain.Errorf(domain.CodeDriverError, "目标网盘未返回秒传试探结果")
		}
		reuse = result.Reuse
		return nil
	})
	return reuse, err
}

type createdTargetDir struct {
	ID       string
	ParentID string
	RelDir   string
}

func markKeptDir(kept map[string]struct{}, relDir string) {
	current := ""
	for _, part := range strings.Split(strings.Trim(relDir, "/"), "/") {
		if part == "" {
			continue
		}
		if current == "" {
			current = part
		} else {
			current += "/" + part
		}
		kept[current] = struct{}{}
	}
}

func (s *Service) cleanupCreatedDirs(ctx context.Context, accountID int64, created []createdTargetDir, kept map[string]struct{}) {
	for _, dir := range removableCreatedRoots(created, kept) {
		if err := s.files.DeleteFiles(ctx, accountID, []string{dir.ID}, dir.ParentID); err != nil {
			s.log.Warn("清理未使用目录失败", "folder_id", dir.ID, "err", err)
		}
	}
}

func removableCreatedRoots(created []createdTargetDir, kept map[string]struct{}) []createdTargetDir {
	createdByRel := make(map[string]createdTargetDir, len(created))
	for _, dir := range created {
		createdByRel[dir.RelDir] = dir
	}
	roots := make([]createdTargetDir, 0, len(created))
	for _, dir := range created {
		if _, ok := kept[dir.RelDir]; ok {
			continue
		}
		parentRel := dir.RelDir
		if index := strings.LastIndexByte(parentRel, '/'); index >= 0 {
			parentRel = parentRel[:index]
		} else {
			parentRel = ""
		}
		if _, parentCreated := createdByRel[parentRel]; parentCreated {
			if _, parentKept := kept[parentRel]; !parentKept {
				continue
			}
		}
		roots = append(roots, dir)
	}
	return roots
}

func EnsureTargetDir(
	ctx context.Context,
	files *file.Service,
	accountID int64,
	rootID, relDir string,
	cache map[string]string,
	createdDirs *[]createdTargetDir,
) (string, error) {
	relDir = strings.Trim(relDir, "/")
	if relDir == "" {
		return rootID, nil
	}
	if id, ok := cache[relDir]; ok {
		return id, nil
	}
	parts := strings.Split(relDir, "/")
	cur := rootID
	curRel := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if curRel == "" {
			curRel = part
		} else {
			curRel = curRel + "/" + part
		}
		if id, ok := cache[curRel]; ok {
			cur = id
			continue
		}
		items, err := files.List(ctx, accountID, cur, false)
		if err != nil {
			return "", err
		}
		found := ""
		for _, it := range items {
			if it.IsDir && it.Name == part {
				found = it.ID
				break
			}
		}
		if found == "" {
			created, err := files.CreateFolder(ctx, accountID, cur, part)
			if err != nil {
				return "", err
			}
			found = created.ID
			if createdDirs != nil {
				*createdDirs = append(*createdDirs, createdTargetDir{ID: found, ParentID: cur, RelDir: curRel})
			}
		}
		cur = found
		cache[curRel] = cur
	}
	cache[relDir] = cur
	return cur, nil
}
