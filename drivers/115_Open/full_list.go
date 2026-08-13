package pan115open

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"litepan/internal/driver"
)

const fullListPageSize = 1150

// ListAllFiles 使用 cur=0 让服务端递归展开 rootID 下全部文件，分页拉取。
// 该模式不返回文件夹，条目自带 pid，由上层结合 pid→路径 缓存还原目录结构。
func (d *Driver) ListAllFiles(ctx context.Context, rootID string) ([]driver.FullListEntry, error) {
	root := d.normalizeParent(rootID)
	var entries []driver.FullListEntry
	offset := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		query := urlValues(map[string]string{
			"cid":      root,
			"limit":    strconv.Itoa(fullListPageSize),
			"offset":   strconv.Itoa(offset),
			"show_dir": "0",
			"cur":      "0",
		})
		var page listPageResp
		if err := d.apiCallFull(ctx, http.MethodGet, pathList, query, nil, &page); err != nil {
			return nil, err
		}
		if len(page.Data) == 0 {
			break
		}
		for _, f := range page.Data {
			if isTrashed(f) {
				continue
			}
			entries = append(entries, driver.FullListEntry{
				FileID:   f.entryID(),
				ParentID: f.parentID(),
				Name:     f.entryName(),
				Size:     f.entrySize(),
				Sha1:     strings.TrimSpace(f.Sha1),
				PickCode: f.pickCode(),
				MTime:    f.entryMTime(),
			})
		}
		offset += len(page.Data)
		if page.Count > 0 && int64(offset) >= page.Count {
			break
		}
		if len(page.Data) < fullListPageSize {
			break
		}
	}
	return entries, nil
}

// ResolveDirPath 通过 /open/folder/get_info 拼出目录完整路径。
// 注意：接口返回的 paths 只是“父目录链”（不含目录自身），必须再追加目录自身名称。
func (d *Driver) ResolveDirPath(ctx context.Context, dirID string) (string, error) {
	id := strings.TrimSpace(dirID)
	if id == "" || id == "0" || id == d.rootID() {
		return "", nil
	}
	query := urlValues(map[string]string{"file_id": id})
	var info fileEntry
	if err := d.apiCall(ctx, http.MethodGet, pathFileInfo, query, nil, &info); err != nil {
		return "", err
	}
	if info.entryID() == "" {
		return "", nil
	}
	return buildDirPath(info.Paths, info.entryName()), nil
}

// buildDirPath 把 get_info 的父目录链（不含自身）与目录自身名称拼成完整路径。
// 父链中 file_id 为 0 的根段跳过；结果不含首尾斜杠。
func buildDirPath(paths []dirPathEntry, selfName string) string {
	segs := make([]string, 0, len(paths)+1)
	for _, p := range paths {
		if strings.TrimSpace(p.FileID.String()) == "0" {
			continue
		}
		if name := strings.TrimSpace(p.FileName); name != "" {
			segs = append(segs, name)
		}
	}
	if name := strings.TrimSpace(selfName); name != "" {
		segs = append(segs, name)
	}
	return strings.Join(segs, "/")
}

var (
	_ driver.FullListLister = (*Driver)(nil)
)
