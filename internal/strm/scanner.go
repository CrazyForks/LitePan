package strm

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"litepan/internal/domain"
	"litepan/internal/file"
	"litepan/internal/playback"
)

var episodeNamePattern = regexp.MustCompile(`(?i)(` +
	`s\d{1,2}[\s._-]*e\d{1,3}|` +
	`s\d{1,2}[\s._-]*ep\d{1,3}|` +
	`(?:^|[^a-z0-9])ep?[\s._-]*\d{1,3}(?:[^a-z0-9]|$)|` +
	`第\s*\d{1,4}\s*[集话話]` +
	`)`)

const defaultExtensions = "mp4;mkv;avi;mov;wmv;flv;ts;m2ts;mpg;mpeg;webm;mp3;flac;aac;wav;m4a;iso"

type ScanSettings struct {
	DefaultExtensions     string
	MinFileSizeMB         int
	ConflictPolicy        string
	MetadataExtensions    string
	MetadataMaxSizeMB     int
	MetadataParentEnabled bool
	ISOFilenameEnabled    bool
}

type ScanDeps struct {
	Files       *file.Service
	Branches    domain.StrmBranchRepository
	Playback    *playback.Service
	StrmDir     string
	BaseURL     string
	Token       string
	SignEnabled bool
	Secret      []byte
	Settings    ScanSettings
	Log         *slog.Logger
	OnProgress  ScanProgressReporter
	Failures    *FailureCollector
}

type ScanResult struct {
	ScannedCount   int64
	GeneratedCount int64
	UpdatedCount   int64
	RemovedCount   int64
	Failures       []ScanFailure
}

type scanScope struct {
	parentID   string
	relDirs    []string
	recursive  bool
	baseEntry  bool
	remotePath string
}

type cleanupScope struct {
	relDirs   []string
	recursive bool
}

type branchScanState struct {
	skippedDirs    map[string]struct{}
	cleanupScopes  []cleanupScope
	remoteChildren map[string]map[string]struct{}
}

func ScanTask(ctx context.Context, task *domain.StrmTask, deps ScanDeps, runMode string) (ScanResult, error) {
	var result ScanResult
	failures := deps.Failures
	if failures == nil {
		failures = NewFailureCollector()
	}
	log := deps.Log
	if log == nil {
		log = slog.Default()
	}
	exts := parseExtensions(task.Extensions)
	if len(exts) == 0 {
		exts = parseExtensions(deps.Settings.DefaultExtensions)
	}
	if len(exts) == 0 {
		exts = parseExtensions(defaultExtensions)
	}
	metaExts := parseExtensions(deps.Settings.MetadataExtensions)
	minMediaBytes := int64(deps.Settings.MinFileSizeMB) * 1024 * 1024
	metaMaxBytes := int64(deps.Settings.MetadataMaxSizeMB) * 1024 * 1024
	if deps.Settings.MetadataMaxSizeMB <= 0 {
		metaMaxBytes = 0
	}
	excludeDirs := parseKeywordRules(task.ExcludeDirKeywords)
	excludeFiles := parseKeywordRules(task.ExcludeFileKeywords)

	root := strings.TrimSpace(deps.StrmDir)
	if root == "" {
		root = "strm"
	}
	useBranch := useBranchScan(runMode, task)
	var allBranches []*domain.StrmBranch
	if useBranch && deps.Branches != nil {
		_, _ = deps.Branches.DeleteExpired(ctx, task.ID)
		var err error
		allBranches, err = deps.Branches.ListByTask(ctx, task.ID)
		if err != nil {
			return result, err
		}
	}
	scopes, branchParentIDs := buildScanScopes(task, allBranches, useBranch)
	if len(scopes) == 0 {
		return result, nil
	}
	if branchParentIDs == nil {
		branchParentIDs = make(map[string]struct{})
	}

	var candidates []mediaCandidate
	var metadataItems []metadataItem
	dirHasMedia := make(map[string]bool)
	subtreeHasMedia := make(map[string]bool)

	state := &branchScanState{
		skippedDirs:    make(map[string]struct{}),
		remoteChildren: make(map[string]map[string]struct{}),
	}

	var monitorScopes, childScopes []scanScope
	for _, scope := range scopes {
		if scope.baseEntry {
			children, remoteNames, err := walkBaseBranchEntry(ctx, task, deps, scope, exts, metaExts, excludeDirs, excludeFiles,
				minMediaBytes, metaMaxBytes, task.SyncMetadata,
				branchParentIDs, state.skippedDirs, root, &candidates, &metadataItems, dirHasMedia, subtreeHasMedia, log)
			if err != nil {
				return result, err
			}
			if remoteNames != nil {
				recordRemoteChildren(state.remoteChildren, scope.relDirs, remoteNames)
			}
			state.cleanupScopes = append(state.cleanupScopes, cleanupScope{relDirs: scope.relDirs, recursive: false})
			childScopes = append(childScopes, children...)
			continue
		}
		monitorScopes = append(monitorScopes, scope)
	}

	skippedParents := removeMonitorBranchesMissingRemote(ctx, deps, allBranches, state.remoteChildren, log)
	for _, scope := range monitorScopes {
		if _, skip := skippedParents[scope.parentID]; skip {
			continue
		}
		state.cleanupScopes = append(state.cleanupScopes, cleanupScope{relDirs: scope.relDirs, recursive: scope.recursive})
		if err := walkScope(ctx, task, deps, scope, exts, metaExts, excludeDirs, excludeFiles,
			minMediaBytes, metaMaxBytes, task.SyncMetadata,
			state.remoteChildren, &candidates, &metadataItems, dirHasMedia, subtreeHasMedia); err != nil {
			return result, err
		}
	}
	for _, scope := range childScopes {
		state.cleanupScopes = append(state.cleanupScopes, cleanupScope{relDirs: scope.relDirs, recursive: true})
		if err := walkScope(ctx, task, deps, scope, exts, metaExts, excludeDirs, excludeFiles,
			minMediaBytes, metaMaxBytes, task.SyncMetadata,
			state.remoteChildren, &candidates, &metadataItems, dirHasMedia, subtreeHasMedia); err != nil {
			return result, err
		}
	}

	selected, _ := selectConflictWinners(candidates, deps.Settings.ConflictPolicy)
	metadataItems = alignMetadataItems(task.OutputFolder, selected, metadataItems, deps.Settings.ISOFilenameEnabled)
	seen := make(map[string]struct{})

	for _, item := range selected {
		result.ScannedCount++
		relPath := LocalRelPath(task.OutputFolder, item.relDirs, item.fileName, deps.Settings.ISOFilenameEnabled)
		if addOversizedPathFailure(failures, ScanFailureStrm, relPath, false) {
			continue
		}
		seen[filepath.ToSlash(relPath)] = struct{}{}
		if _, err := MigrateLegacyISOStrmFile(root, task.OutputFolder, item.relDirs, item.fileName, item.fileID, deps.Settings.ISOFilenameEnabled); err != nil {
			failures.Add(ScanFailureStrm, filepath.ToSlash(relPath), err.Error())
			continue
		}
		if task.ScanMode == domain.StrmScanModeIncrementalMissing {
			if _, err := os.Stat(filepath.Join(root, relPath)); err == nil {
				continue
			}
		}
		url := BuildPlayURL(deps.BaseURL, task.AccountID, item.fileID, item.fileName, deps.Token, deps.SignEnabled, deps.Secret)
		created, updated, err := writeStrmFile(root, relPath, url, task.ScanMode)
		if err != nil {
			failures.Add(ScanFailureStrm, filepath.ToSlash(relPath), err.Error())
			continue
		}
		if created {
			result.GeneratedCount++
		} else if updated {
			result.UpdatedCount++
		}
	}

	var filteredMetadata []metadataItem
	if task.SyncMetadata && len(metadataItems) > 0 {
		filteredMetadata = filterMetadataItems(metadataItems, dirHasMedia, subtreeHasMedia, deps.Settings.MetadataParentEnabled)
		syncer := &metadataSyncer{playback: deps.Playback, failures: failures, onProgress: deps.OnProgress}
		n, err := syncer.syncFiles(ctx, task.AccountID, root, filteredMetadata)
		if err != nil {
			return result, err
		}
		result.GeneratedCount += n
	}

	if task.ScanMode == domain.StrmScanModeIncrementalUpdate || task.ScanMode == domain.StrmScanModeFullSync {
		var removed int64
		var err error
		if useBranch && len(state.cleanupScopes) > 0 {
			removed, err = cleanupScopedStaleFiles(root, task.OutputFolder, seen, state.cleanupScopes, state.skippedDirs, failures)
		} else {
			removed, err = cleanupScopedStaleFiles(root, task.OutputFolder, seen, []cleanupScope{{relDirs: nil, recursive: true}}, nil, failures)
		}
		if err != nil {
			return result, err
		}
		n, err := cleanupMissingRemoteChildDirs(root, task.OutputFolder, state.remoteChildren, failures, log)
		if err != nil {
			return result, err
		}
		result.RemovedCount = removed + n
	}

	log.Debug("strm scan finished",
		"task_id", task.ID,
		"scanned", result.ScannedCount,
		"generated", result.GeneratedCount,
		"updated", result.UpdatedCount,
		"removed", result.RemovedCount,
		"failures", failures.Len(),
	)
	result.Failures = failures.Items()
	return result, nil
}

func useBranchScan(runMode string, task *domain.StrmTask) bool {
	return runMode == domain.StrmRunModeBranch ||
		(runMode != domain.StrmRunModeFull && task.BranchCheckEnabled)
}

func buildScanScopes(task *domain.StrmTask, branches []*domain.StrmBranch, useBranch bool) ([]scanScope, map[string]struct{}) {
	if useBranch && len(branches) > 0 {
		parentIDs := make(map[string]struct{}, len(branches))
		scopes := make([]scanScope, 0, len(branches))
		for _, b := range branches {
			parentIDs[b.ParentID] = struct{}{}
			rel := splitRelativePath(b.RelativePath)
			isBase := b.BranchType == domain.StrmBranchTypeBase
			recursive := b.Recursive
			if isBase {
				recursive = false
			}
			scopes = append(scopes, scanScope{
				parentID:   b.ParentID,
				relDirs:    rel,
				recursive:  recursive,
				baseEntry:  isBase,
				remotePath: strings.TrimRight(strings.TrimSpace(b.Path), "/"),
			})
		}
		return scopes, parentIDs
	}
	parentID := strings.TrimSpace(task.ParentID)
	if parentID == "" {
		parentID = "0"
	}
	return []scanScope{{parentID: parentID, recursive: task.Recursive}}, nil
}

func splitRelativePath(rel string) []string {
	rel = strings.Trim(strings.TrimSpace(rel), "/")
	if rel == "" {
		return nil
	}
	return strings.Split(rel, "/")
}

func hasLocalStrmUnder(root, outputFolder string, relDirs []string) bool {
	localRoot := filepath.Join(root, SafeName(outputFolder))
	for _, dir := range relDirs {
		localRoot = filepath.Join(localRoot, SafeName(dir))
	}
	info, err := os.Stat(localRoot)
	if err != nil || !info.IsDir() {
		return false
	}
	found := false
	_ = filepath.WalkDir(localRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(d.Name()), ".strm") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

func looksLikeEpisodeFile(name string, exts map[string]struct{}) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if _, ok := exts[ext]; !ok {
		return false
	}
	return episodeNamePattern.MatchString(name)
}

func shouldAutoAddTemporaryBranch(ctx context.Context, deps ScanDeps, task *domain.StrmTask, folderID string, exts map[string]struct{}) bool {
	if deps.Files == nil {
		return false
	}
	items, err := deps.Files.List(ctx, task.AccountID, folderID, false)
	if err != nil {
		return false
	}
	for i := range items {
		item := items[i]
		if item.IsDir {
			return true
		}
		if looksLikeEpisodeFile(item.Name, exts) {
			return true
		}
	}
	return false
}

func walkBaseBranchEntry(
	ctx context.Context,
	task *domain.StrmTask,
	deps ScanDeps,
	scope scanScope,
	exts, metaExts map[string]struct{},
	excludeDirs, excludeFiles []string,
	minMediaBytes, metaMaxBytes int64,
	syncMetadata bool,
	branchParentIDs map[string]struct{},
	skippedDirs map[string]struct{},
	strmRoot string,
	candidates *[]mediaCandidate,
	metadataItems *[]metadataItem,
	dirHasMedia, subtreeHasMedia map[string]bool,
	log *slog.Logger,
) ([]scanScope, map[string]struct{}, error) {
	relDirs := append([]string{}, scope.relDirs...)
	reportScanProgress(deps.OnProgress, ScanPhaseScan, 0, 0, dirProgressLabel(relDirs))
	items, err := deps.Files.List(ctx, task.AccountID, scope.parentID, false)
	if err != nil {
		return nil, nil, err
	}

	currentKey := dirKey(relDirs)
	localHasMedia := false
	var dirMeta []metadataItem
	var childScopes []scanScope
	remoteChildNames := make(map[string]struct{})

	for i := range items {
		item := items[i]
		name := item.Name
		if item.IsDir {
			remoteChildNames[SafeName(name)] = struct{}{}
			if matchesKeywordRules(name, excludeDirs) {
				continue
			}
			childID := item.ID
			if _, known := branchParentIDs[childID]; known {
				continue
			}
			childRel := append(append([]string{}, relDirs...), name)
			if hasLocalStrmUnder(strmRoot, task.OutputFolder, childRel) {
				skippedDirs[dirKey(childRel)] = struct{}{}
				markSubtreeMedia(subtreeHasMedia, childRel)
				continue
			}
			childRemote := joinRemotePath(scope.remotePath, name)
			childScope := scanScope{
				parentID:   childID,
				relDirs:    childRel,
				recursive:  true,
				remotePath: childRemote,
			}
			if shouldAutoAddTemporaryBranch(ctx, deps, task, childID, exts) && deps.Branches != nil {
				relativePath := strings.Join(childRel, "/")
				expiresAt := time.Now().Add(30 * 24 * time.Hour)
				branch := &domain.StrmBranch{
					TaskID:        task.ID,
					AccountID:     task.AccountID,
					ParentID:      childID,
					Path:          childRemote,
					RelativePath:  relativePath,
					Recursive:     true,
					RetentionDays: 30,
					ExpiresAt:     expiresAt,
					BranchType:    domain.StrmBranchTypeTemporary,
					Source:        "auto",
					Status:        "running",
				}
				if _, createErr := deps.Branches.Create(ctx, branch); createErr == nil {
					branchParentIDs[childID] = struct{}{}
					if log != nil {
						log.Info("strm auto temporary branch", "path", childRemote)
					}
				}
			}
			childScopes = append(childScopes, childScope)
			continue
		}
		if matchesKeywordRules(name, excludeFiles) {
			continue
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
		if _, ok := exts[ext]; ok {
			if minMediaBytes > 0 && item.Size < minMediaBytes {
				continue
			}
			*candidates = append(*candidates, mediaCandidate{
				fileID: item.ID, fileName: name, size: item.Size, relDirs: append([]string{}, relDirs...),
			})
			if deps.OnProgress != nil {
				reportScanProgress(deps.OnProgress, ScanPhaseScan, 0, 1, dirProgressLabel(relDirs))
			}
			localHasMedia = true
			continue
		}
		if syncMetadata && len(metaExts) > 0 {
			if _, ok := metaExts[ext]; ok {
				if metaMaxBytes > 0 && item.Size > metaMaxBytes {
					continue
				}
				dirMeta = append(dirMeta, newMetadataItem(item.ID, name, task.OutputFolder, relDirs))
			}
		}
	}
	if localHasMedia {
		dirHasMedia[currentKey] = true
		markSubtreeMedia(subtreeHasMedia, relDirs)
	}
	if len(dirMeta) > 0 {
		*metadataItems = append(*metadataItems, dirMeta...)
	}
	if deps.OnProgress != nil {
		reportScanProgress(deps.OnProgress, ScanPhaseScan, 1, 0, dirProgressLabel(relDirs))
	}
	return childScopes, remoteChildNames, nil
}

func joinRemotePath(base, name string) string {
	seg := SafeName(name)
	if base == "" {
		return "/" + seg
	}
	return base + "/" + seg
}

func walkScope(
	ctx context.Context,
	task *domain.StrmTask,
	deps ScanDeps,
	scope scanScope,
	exts, metaExts map[string]struct{},
	excludeDirs, excludeFiles []string,
	minMediaBytes, metaMaxBytes int64,
	syncMetadata bool,
	remoteChildren map[string]map[string]struct{},
	candidates *[]mediaCandidate,
	metadataItems *[]metadataItem,
	dirHasMedia, subtreeHasMedia map[string]bool,
) error {
	type node struct {
		parentID string
		relDirs  []string
	}
	stack := []node{{parentID: scope.parentID, relDirs: append([]string{}, scope.relDirs...)}}
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		reportScanProgress(deps.OnProgress, ScanPhaseScan, 0, 0, dirProgressLabel(n.relDirs))
		items, err := deps.Files.List(ctx, task.AccountID, n.parentID, false)
		if err != nil {
			return err
		}
		childNames := make(map[string]struct{})
		dirKey := dirKey(n.relDirs)
		localHasMedia := false
		var dirMeta []metadataItem
		for i := range items {
			item := items[i]
			name := item.Name
			if item.IsDir {
				childNames[SafeName(name)] = struct{}{}
				if matchesKeywordRules(name, excludeDirs) {
					continue
				}
				if scope.recursive {
					childDirs := append(append([]string{}, n.relDirs...), name)
					stack = append(stack, node{parentID: item.ID, relDirs: childDirs})
				}
				continue
			}
			if matchesKeywordRules(name, excludeFiles) {
				continue
			}
			ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
			if _, ok := exts[ext]; ok {
				if minMediaBytes > 0 && item.Size < minMediaBytes {
					continue
				}
				*candidates = append(*candidates, mediaCandidate{
					fileID: item.ID, fileName: name, size: item.Size, relDirs: append([]string{}, n.relDirs...),
				})
				if deps.OnProgress != nil {
					reportScanProgress(deps.OnProgress, ScanPhaseScan, 0, 1, dirProgressLabel(n.relDirs))
				}
				localHasMedia = true
				continue
			}
			if syncMetadata && len(metaExts) > 0 {
				if _, ok := metaExts[ext]; ok {
					if metaMaxBytes > 0 && item.Size > metaMaxBytes {
						continue
					}
					dirMeta = append(dirMeta, newMetadataItem(item.ID, name, task.OutputFolder, n.relDirs))
				}
			}
		}
		recordRemoteChildren(remoteChildren, n.relDirs, childNames)
		if localHasMedia {
			dirHasMedia[dirKey] = true
			markSubtreeMedia(subtreeHasMedia, n.relDirs)
		}
		if len(dirMeta) > 0 {
			*metadataItems = append(*metadataItems, dirMeta...)
		}
		if deps.OnProgress != nil {
			reportScanProgress(deps.OnProgress, ScanPhaseScan, 1, 0, dirProgressLabel(n.relDirs))
		}
	}
	return nil
}

func markSubtreeMedia(m map[string]bool, relDirs []string) {
	for i := 0; i <= len(relDirs); i++ {
		m[dirKey(relDirs[:i])] = true
	}
}

func filterMetadataItems(items []metadataItem, dirHasMedia, subtreeHasMedia map[string]bool, parentEnabled bool) []metadataItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]metadataItem, 0, len(items))
	seen := make(map[string]int)
	for _, item := range items {
		key := dirKey(item.relDirs)
		if !dirHasMedia[key] && !(parentEnabled && subtreeHasMedia[key]) {
			continue
		}
		if index, ok := seen[item.relPath]; ok {
			if item.direct && !out[index].direct {
				out[index] = item
			}
			continue
		}
		seen[item.relPath] = len(out)
		out = append(out, item)
	}
	return out
}

func writeStrmFile(root, relPath, url, scanMode string) (created, updated bool, err error) {
	fullPath := filepath.Join(root, relPath)
	_, statErr := os.Stat(fullPath)
	exists := statErr == nil

	switch scanMode {
	case domain.StrmScanModeIncrementalMissing:
		if exists {
			return false, false, nil
		}
		if err := WriteStrmFile(root, relPath, url); err != nil {
			return false, false, err
		}
		return true, false, nil

	case domain.StrmScanModeFullSync:
		if err := WriteStrmFile(root, relPath, url); err != nil {
			return false, false, err
		}
		if exists {
			return false, true, nil
		}
		return true, false, nil

	default:
		if exists {
			old, readErr := os.ReadFile(fullPath)
			if readErr == nil && strings.TrimSpace(string(old)) == strings.TrimSpace(url) {
				return false, false, nil
			}
			if err := WriteStrmFile(root, relPath, url); err != nil {
				return false, false, err
			}
			return false, true, nil
		}
		if err := WriteStrmFile(root, relPath, url); err != nil {
			return false, false, err
		}
		return true, false, nil
	}
}

func parseExtensions(raw string) map[string]struct{} {
	raw = strings.ReplaceAll(raw, ",", ";")
	parts := strings.Split(raw, ";")
	out := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(strings.TrimPrefix(p, ".")))
		if p == "" {
			continue
		}
		out[p] = struct{}{}
	}
	return out
}

func recordRemoteChildren(remoteChildren map[string]map[string]struct{}, relDirs []string, names map[string]struct{}) {
	if remoteChildren == nil || len(names) == 0 {
		return
	}
	key := dirKey(relDirs)
	if remoteChildren[key] == nil {
		remoteChildren[key] = make(map[string]struct{}, len(names))
	}
	for name := range names {
		remoteChildren[key][name] = struct{}{}
	}
}

func relDirsFromDirKey(key string) []string {
	key = strings.Trim(key, "/")
	if key == "" {
		return nil
	}
	return strings.Split(key, "/")
}

func localTaskDir(root, outputFolder string, relDirs []string) string {
	local := filepath.Join(root, SafeName(outputFolder))
	for _, dir := range relDirs {
		local = filepath.Join(local, dir)
	}
	return local
}

func isMetadataExtension(name string, metaExts map[string]struct{}) bool {
	if len(metaExts) == 0 {
		return false
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	_, ok := metaExts[ext]
	return ok
}

// removeStaleStrmAndSameStemSidecars 删除过期 STRM 及同主干旁路文件，不处理目录级元数据。
func removeStaleStrmAndSameStemSidecars(strmPath string) error {
	if err := os.Remove(strmPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	base := filepath.Base(strmPath)
	ext := filepath.Ext(base)
	if ext == "" {
		return nil
	}
	stem := base[:len(base)-len(ext)]
	if stem == "" {
		return nil
	}
	dir := filepath.Dir(strmPath)
	names := []string{stem + ".nfo"}
	for _, img := range []string{".jpg", ".jpeg", ".png", ".webp", ".bmp", ".gif"} {
		names = append(names, stem+img, stem+"-poster"+img, stem+"-thumb"+img)
	}
	for _, name := range names {
		p := filepath.Join(dir, name)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// cleanupScopedStaleFiles 清理过期 .strm，并顺带删除同主干旁路元数据。
func cleanupScopedStaleFiles(root, outputFolder string, seen map[string]struct{}, scopes []cleanupScope, skipped map[string]struct{}, failures *FailureCollector) (int64, error) {
	taskFolder := SafeName(outputFolder)
	var removed int64
	for _, sc := range scopes {
		cleanupRoot := filepath.Join(root, taskFolder)
		cleanupRel := taskFolder
		for _, dir := range sc.relDirs {
			safeDir := SafeName(dir)
			cleanupRoot = filepath.Join(cleanupRoot, safeDir)
			cleanupRel = filepath.Join(cleanupRel, safeDir)
		}
		if addOversizedPathFailure(failures, ScanFailureStrm, cleanupRel, true) {
			continue
		}
		if _, err := os.Stat(cleanupRoot); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, err
		}
		if sc.recursive {
			err := filepath.WalkDir(cleanupRoot, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				name := d.Name()
				if !strings.EqualFold(filepath.Ext(name), ".strm") {
					return nil
				}
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				rel = filepath.ToSlash(rel)
				if _, ok := seen[rel]; ok {
					return nil
				}
				if isStrmUnderSkipped(rel, taskFolder, skipped) {
					return nil
				}
				if err := removeStaleStrmAndSameStemSidecars(path); err != nil {
					return err
				}
				removed++
				return nil
			})
			if err != nil {
				return removed, err
			}
			_ = removeEmptyDirs(cleanupRoot)
			continue
		}
		entries, err := os.ReadDir(cleanupRoot)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.EqualFold(filepath.Ext(name), ".strm") {
				continue
			}
			full := filepath.Join(cleanupRoot, name)
			rel, err := filepath.Rel(root, full)
			if err != nil {
				return removed, err
			}
			rel = filepath.ToSlash(rel)
			if _, ok := seen[rel]; ok {
				continue
			}
			if isStrmUnderSkipped(rel, taskFolder, skipped) {
				continue
			}
			if err := removeStaleStrmAndSameStemSidecars(full); err != nil {
				return removed, err
			}
			removed++
		}
		_ = removeEmptyDirs(cleanupRoot)
	}
	return removed, nil
}

func cleanupMissingRemoteChildDirs(root, outputFolder string, remoteChildren map[string]map[string]struct{}, failures *FailureCollector, log *slog.Logger) (int64, error) {
	if len(remoteChildren) == 0 {
		return 0, nil
	}
	taskFolder := SafeName(outputFolder)
	var removed int64
	for parentKey, remoteNames := range remoteChildren {
		relDirs := relDirsFromDirKey(parentKey)
		localRel := localTaskDir("", taskFolder, relDirs)
		if addOversizedPathFailure(failures, ScanFailureStrm, localRel, true) {
			continue
		}
		localBase := localTaskDir(root, taskFolder, relDirs)
		entries, err := os.ReadDir(localBase)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if _, ok := remoteNames[SafeName(e.Name())]; ok {
				continue
			}
			childPath := filepath.Join(localBase, e.Name())
			n := countFilesUnder(childPath)
			if err := os.RemoveAll(childPath); err != nil && !os.IsNotExist(err) {
				return removed, err
			}
			removed += n
			if log != nil {
				log.Info("strm cleanup remote deleted dir", "path", childPath, "files_removed", n)
			}
		}
	}
	return removed, nil
}

func removeMonitorBranchesMissingRemote(
	ctx context.Context,
	deps ScanDeps,
	branches []*domain.StrmBranch,
	baseRemote map[string]map[string]struct{},
	log *slog.Logger,
) map[string]struct{} {
	skipped := make(map[string]struct{})
	if deps.Branches == nil || len(baseRemote) == 0 {
		return skipped
	}
	var baseBranches []*domain.StrmBranch
	for _, b := range branches {
		if b.BranchType == domain.StrmBranchTypeBase {
			baseBranches = append(baseBranches, b)
		}
	}
	for _, b := range branches {
		if b.BranchType == domain.StrmBranchTypeBase {
			continue
		}
		if !monitorBranchMissingOnRemote(b, baseBranches, baseRemote) {
			continue
		}
		if err := deps.Branches.Delete(ctx, b.ID); err != nil {
			if log != nil {
				log.Warn("strm remove stale monitor branch failed", "path", b.Path, "err", err)
			}
			continue
		}
		skipped[b.ParentID] = struct{}{}
		if log != nil {
			log.Info("strm remove stale monitor branch", "path", b.Path)
		}
	}
	return skipped
}

func monitorBranchMissingOnRemote(branch *domain.StrmBranch, bases []*domain.StrmBranch, baseRemote map[string]map[string]struct{}) bool {
	branchRel := splitRelativePath(branch.RelativePath)
	for _, base := range bases {
		baseRel := splitRelativePath(base.RelativePath)
		child, ok := firstChildUnderBase(branchRel, baseRel)
		if !ok {
			continue
		}
		remoteNames, listed := baseRemote[dirKey(baseRel)]
		if !listed {
			continue
		}
		if _, exists := remoteNames[child]; !exists {
			return true
		}
		return false
	}
	return false
}

func firstChildUnderBase(branchRel, baseRel []string) (string, bool) {
	if len(branchRel) == 0 {
		return "", false
	}
	if len(baseRel) == 0 {
		return SafeName(branchRel[0]), true
	}
	if len(branchRel) <= len(baseRel) {
		return "", false
	}
	for i := range baseRel {
		if SafeName(branchRel[i]) != SafeName(baseRel[i]) {
			return "", false
		}
	}
	return SafeName(branchRel[len(baseRel)]), true
}

func isStrmUnderSkipped(strmRel, taskFolder string, skipped map[string]struct{}) bool {
	if len(skipped) == 0 {
		return false
	}
	rel := filepath.ToSlash(strmRel)
	prefix := SafeName(taskFolder)
	if !strings.HasPrefix(rel, prefix+"/") && rel != prefix {
		return false
	}
	suffix := strings.TrimPrefix(rel, prefix+"/")
	for key := range skipped {
		if key == "" {
			continue
		}
		if suffix == key || strings.HasPrefix(suffix, key+"/") {
			return true
		}
	}
	return false
}

func countFilesUnder(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func countStrmFiles(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(d.Name()), ".strm") {
			n++
		}
		return nil
	})
	return n
}

func removeEmptyDirs(root string) error {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) > len(dirs[j])
	})
	for _, path := range dirs {
		entries, readErr := os.ReadDir(path)
		if readErr != nil {
			continue
		}
		if len(entries) == 0 {
			_ = os.Remove(path)
		}
	}
	return nil
}
