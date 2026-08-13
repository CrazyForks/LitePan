package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"litepan/internal/domain"
	"litepan/internal/settings"
	"litepan/internal/upload"
)

type localUploadMapping struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

var (
	systemJunkFiles = map[string]struct{}{
		".ds_store":   {},
		".localized":  {},
		"thumbs.db":   {},
		"desktop.ini": {},
	}
	systemJunkDirs = map[string]struct{}{
		"__macosx":                  {},
		".spotlight-v100":           {},
		".trashes":                  {},
		".fseventsd":                {},
		"$recycle.bin":              {},
		"system volume information": {},
	}
	systemTrashDirPattern = regexp.MustCompile(`^\.trash-\d+$`)
)

func isSystemJunkFile(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if _, ok := systemJunkFiles[n]; ok {
		return true
	}
	return strings.HasPrefix(n, "._")
}

func isSystemJunkDir(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if _, ok := systemJunkDirs[n]; ok {
		return true
	}
	return systemTrashDirPattern.MatchString(n)
}

func (h *Handler) loadLocalUploadMappings() []localUploadMapping {
	if h.settings == nil {
		return nil
	}
	raw := h.settings.String(settings.KeyLocalUploadMappings)
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []localUploadMapping
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	// 只保留合法项
	valid := out[:0]
	seen := make(map[string]struct{}, len(out))
	for _, m := range out {
		name := strings.TrimSpace(m.Name)
		path := strings.TrimSpace(m.Path)
		if name == "" || path == "" || !strings.HasPrefix(path, "/") {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		valid = append(valid, localUploadMapping{Name: name, Path: filepath.Clean(path)})
	}
	return valid
}

func (h *Handler) findLocalUploadMapping(name string) (localUploadMapping, bool) {
	for _, m := range h.loadLocalUploadMappings() {
		if m.Name == strings.TrimSpace(name) {
			return m, true
		}
	}
	return localUploadMapping{}, false
}

func (h *Handler) getLocalUploadConfig(w http.ResponseWriter, _ *http.Request) {
	enabled := false
	if h.settings != nil {
		enabled = h.settings.Bool(settings.KeyLocalUploadEnabled)
	}
	writeOK(w, map[string]any{
		"enabled":  enabled,
		"mappings": h.loadLocalUploadMappings(),
	})
}

func (h *Handler) updateLocalUploadConfig(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Enabled  bool                 `json:"enabled"`
		Mappings []localUploadMapping `json:"mappings"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if h.settings == nil {
		writeErr(w, domain.Errorf(domain.CodeInternal, "设置服务未初始化"))
		return
	}
	seen := make(map[string]struct{}, len(in.Mappings))
	for i := range in.Mappings {
		m := &in.Mappings[i]
		m.Name = strings.TrimSpace(m.Name)
		m.Path = strings.TrimSpace(m.Path)
		if m.Name == "" {
			writeErr(w, domain.Errorf(domain.CodeValidation, "映射标签名不能为空"))
			return
		}
		if _, dup := seen[m.Name]; dup {
			writeErr(w, domain.Errorf(domain.CodeValidation, "映射标签名重复：%s", m.Name))
			return
		}
		seen[m.Name] = struct{}{}
		if m.Path == "" || !strings.HasPrefix(m.Path, "/") {
			writeErr(w, domain.Errorf(domain.CodeValidation, "映射路径必须是以 / 开头的容器内路径：%s", m.Path))
			return
		}
		cleaned := filepath.Clean(m.Path)
		if cleaned == "/" || cleaned != m.Path {
			writeErr(w, domain.Errorf(domain.CodeValidation, "映射路径不合法：%s", m.Path))
			return
		}
		m.Path = cleaned
	}
	raw, err := json.Marshal(in.Mappings)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.settings.Update(r.Context(), map[string]string{
		settings.KeyLocalUploadEnabled:  boolString(in.Enabled),
		settings.KeyLocalUploadMappings: string(raw),
	}); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, map[string]any{
		"enabled":  in.Enabled,
		"mappings": in.Mappings,
	})
}

type localUploadEntry struct {
	Name    string `json:"name"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	MTime   int64  `json:"mtime"`
	RelPath string `json:"rel_path"`
}

func (h *Handler) browseLocalUpload(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mapping string `json:"mapping"`
		Path    string `json:"path"` // 相对映射根的目录，空表示根
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	m, ok := h.findLocalUploadMapping(in.Mapping)
	if !ok {
		writeErr(w, domain.Errorf(domain.CodeValidation, "映射目录不存在：%s", in.Mapping))
		return
	}
	rel := cleanRelativePath(in.Path)
	dir := filepath.Join(m.Path, rel)
	if !isWithinRoot(dir, m.Path) {
		writeErr(w, domain.Errorf(domain.CodeValidation, "路径超出映射目录范围"))
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeErr(w, domain.Errorf(domain.CodeDriverError, "读取目录失败：%v", err))
		return
	}
	out := make([]localUploadEntry, 0, len(entries))
	for _, e := range entries {
		if isSystemJunkDir(e.Name()) || isSystemJunkFile(e.Name()) {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue
		}
		itemRel := e.Name()
		if rel != "" {
			itemRel = rel + "/" + e.Name()
		}
		out = append(out, localUploadEntry{
			Name:    e.Name(),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			MTime:   info.ModTime().Unix(),
			RelPath: itemRel,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	writeOK(w, map[string]any{"mapping": in.Mapping, "path": rel, "items": out})
}

func (h *Handler) createLocalUploadTasks(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AccountID      int64  `json:"account_id"`
		Mapping        string `json:"mapping"`
		TargetPath     string `json:"target_path"`
		TargetDisplay  string `json:"target_display_path"`
		ConflictPolicy string `json:"conflict_policy"`
		ClientTaskID   string `json:"client_task_id"`
		DisplayName    string `json:"display_name"`
		Items          []struct {
			RelPath string `json:"rel_path"`
			IsDir   bool   `json:"is_dir"`
		} `json:"items"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	if in.AccountID <= 0 {
		writeErr(w, domain.Errorf(domain.CodeValidation, "非法 account_id"))
		return
	}
	if len(in.Items) == 0 {
		writeErr(w, domain.Errorf(domain.CodeValidation, "未选择任何文件"))
		return
	}
	m, ok := h.findLocalUploadMapping(in.Mapping)
	if !ok {
		writeErr(w, domain.Errorf(domain.CodeValidation, "映射目录不存在：%s", in.Mapping))
		return
	}
	conflict := strings.TrimSpace(in.ConflictPolicy)
	if conflict == "" {
		conflict = "overwrite"
	}
	var sources []localUploadSource
	for _, item := range in.Items {
		rel := cleanRelativePath(item.RelPath)
		if rel == "" {
			continue
		}
		abs := filepath.Join(m.Path, rel)
		if !isWithinRoot(abs, m.Path) {
			writeErr(w, domain.Errorf(domain.CodeValidation, "路径超出映射目录范围：%s", item.RelPath))
			return
		}
		if item.IsDir {
			folderRel := strings.Trim(rel, "/")
			if err := filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					if isSystemJunkDir(d.Name()) {
						return filepath.SkipDir
					}
					return nil
				}
				if isSystemJunkFile(d.Name()) {
					return nil
				}
				relDir := folderRel
				if dir := filepath.Dir(p); dir != abs {
					inner, rerr := filepath.Rel(abs, dir)
					if rerr != nil {
						return rerr
					}
					if inner != "." {
						relDir = filepath.ToSlash(filepath.Join(relDir, inner))
					}
				}
				sources = append(sources, localUploadSource{abs: p, relDir: relDir})
				return nil
			}); err != nil {
				writeErr(w, domain.Errorf(domain.CodeDriverError, "遍历目录失败：%v", err))
				return
			}
			continue
		}
		if isSystemJunkFile(filepath.Base(abs)) {
			continue
		}
		sources = append(sources, localUploadSource{abs: abs})
	}
	if len(sources) == 0 {
		writeErr(w, domain.Errorf(domain.CodeValidation, "未找到可上传的文件"))
		return
	}

	// 异步批量创建任务：接口立即返回，前端乐观显示占位批次，避免界面卡在创建中。
	ctx := context.WithoutCancel(r.Context())
	go h.createLocalUploadTasksSync(ctx, m, in.AccountID, in.TargetPath,
		strings.TrimSpace(in.TargetDisplay), in.ClientTaskID, in.DisplayName, conflict, sources)

	writeOK(w, map[string]any{"accepted": true, "count": len(sources)})
}

// createLocalUploadTasksSync 后台批量创建上传任务（分批 CreateBatch）。
func (h *Handler) createLocalUploadTasksSync(
	ctx context.Context,
	m localUploadMapping,
	accountID int64,
	targetRoot, targetDisplay, clientTaskID, displayName, conflict string,
	sources []localUploadSource,
) ([]*upload.Task, error) {
	const batchSize = 100
	batch := make([]upload.CreateParams, 0, batchSize)
	seq := 0
	var tasks []*upload.Task
	accountName, driverType := "", ""
	if h.accountSvc != nil {
		accountName, driverType, _ = h.accountSvc.LookupUploadAccount(ctx, accountID)
	}
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if h.uploads == nil {
			h.logError("上传服务未初始化，本机上传任务创建失败", "count", len(batch))
			batch = batch[:0]
			return
		}
		created, err := h.uploads.CreateBatch(ctx, batch)
		if err != nil {
			h.logError("批量创建本机上传任务失败", "count", len(batch), "err", err.Error())
		} else {
			tasks = append(tasks, created...)
		}
		batch = batch[:0]
	}
	for _, s := range sources {
		if err := ctx.Err(); err != nil {
			return tasks, err
		}
		targetParent := targetRoot
		if s.relDir != "" {
			parent, err := h.ensureLocalUploadTargetDir(ctx, accountID, targetRoot, s.relDir)
			if err != nil {
				h.logError("创建网盘子目录失败", "dir", s.relDir, "err", err.Error())
				continue
			}
			targetParent = parent
		}
		info, err := statLocalFile(s.abs)
		if err != nil {
			h.logError("读取本地文件失败", "path", s.abs, "err", err.Error())
			continue
		}
		taskID := clientTaskID
		if taskID != "" {
			taskID = taskID + "-" + strconv.Itoa(seq)
		}
		batch = append(batch, upload.CreateParams{
			ClientTaskID:      taskID,
			AccountID:         accountID,
			AccountName:       accountName,
			DriverType:        driverType,
			FileName:          filepath.Base(s.abs),
			DisplayName:       displayName,
			TargetPath:        targetParent,
			TargetDisplayPath: joinLocalDisplayPath(targetDisplay, s.relDir),
			LocalPath:         s.abs,
			TotalBytes:        info.Size(),
			ConflictPolicy:    conflict,
			CleanupLocalMode:  upload.CleanupLocalModeKeep,
		})
		seq++
		if len(batch) >= batchSize {
			flush()
		}
	}
	flush()
	return tasks, nil
}

type localUploadSource struct {
	abs    string
	relDir string
}

func statLocalFile(abs string) (fs.FileInfo, error) {
	return os.Stat(abs)
}

func joinLocalDisplayPath(base, relDir string) string {
	base = strings.Trim(strings.ReplaceAll(base, "\\", "/"), "/")
	relDir = strings.Trim(strings.ReplaceAll(relDir, "\\", "/"), "/")
	if relDir == "" {
		return base
	}
	if base == "" {
		return relDir
	}
	return base + "/" + relDir
}

func (h *Handler) logError(msg string, args ...any) {
	if h.log != nil {
		h.log.Error(msg, args...)
		return
	}
	slog.Error(msg, args...)
}

// ensureLocalUploadTargetDir 从根目录 ID 开始逐级解析/创建网盘子目录，返回最终目录 ID。
func (h *Handler) ensureLocalUploadTargetDir(ctx context.Context, accountID int64, rootID, relDir string) (string, error) {
	if h.files == nil {
		return "", domain.Errorf(domain.CodeInternal, "文件服务未就绪")
	}
	relDir = strings.Trim(strings.ReplaceAll(relDir, "\\", "/"), "/")
	if relDir == "" {
		return rootID, nil
	}
	cur := rootID
	for _, part := range strings.Split(relDir, "/") {
		if part == "" {
			continue
		}
		items, err := h.files.List(ctx, accountID, cur, false)
		if err != nil {
			return "", err
		}
		next := ""
		for _, item := range items {
			if item.IsDir && item.Name == part {
				next = item.ID
				break
			}
		}
		if next == "" {
			created, err := h.files.CreateFolder(ctx, accountID, cur, part)
			if err != nil {
				return "", err
			}
			next = created.ID
		}
		cur = next
	}
	return cur, nil
}

func cleanRelativePath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	p = strings.Trim(p, "/")
	if p == "" {
		return ""
	}
	cleaned := filepath.Clean(filepath.FromSlash(p))
	if cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return ""
	}
	return filepath.ToSlash(cleaned)
}

func isWithinRoot(abs, root string) bool {
	root = filepath.Clean(root)
	abs = filepath.Clean(abs)
	if abs == root {
		return true
	}
	return strings.HasPrefix(abs, root+string(filepath.Separator))
}

func boolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
