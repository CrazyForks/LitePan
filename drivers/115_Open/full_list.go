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
// 完整性策略：第一页即拿到接口 Count 作为总数，按“页数 = 总数/页大小”固定拉满每一页，
// 不再用“本页不足页大小就提前停”的短页判断——115 单页可能因厂商缓存/并发滞后返回不足
// 一页但后面仍有数据，短页提前停会导致清单不完整，进而让上层把未扫到的目录误判为已删除。
// 每页仍以空页作为兜底终点，避免依赖不可信的 Count 死循环。
func (d *Driver) ListAllFiles(ctx context.Context, rootID string) ([]driver.FullListEntry, error) {
	root := d.normalizeParent(rootID)
	var entries []driver.FullListEntry
	offset := 0
	total := int64(-1)
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
		if total < 0 && page.Count > 0 {
			total = page.Count
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
		if total > 0 && int64(offset) >= total {
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
