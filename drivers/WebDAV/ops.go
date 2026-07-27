package webdav

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"

	"litepan/internal/domain"
	"litepan/internal/driver"
	"litepan/internal/httpx"
)


// WebDAV GET 需要认证，无法安全 302 暴露凭据，一律走本机代理
func (d *Driver) ResolveDownload(ctx context.Context, req driver.DownloadRequest) (*domain.DownloadInfo, error) {
	c, err := d.ensureClient()
	if err != nil {
		return nil, err
	}
	p := d.normalizePath(req.FileID)
	_ = ctx
	fi, err := c.Stat(p)
	if err != nil {
		return nil, mapError(err)
	}
	if fi.IsDir() {
		return nil, domain.Errorf(domain.CodeValidation, "目录不支持下载")
	}

	url := d.resourceURL(p)
	ua := strings.TrimSpace(req.UA)
	if ua == "" {
		ua = httpx.DefaultUserAgent
	}
	headers := http.Header{
		"Authorization":   []string{"Basic " + basicAuth(d.add.Username, d.add.Password)},
		"User-Agent":      []string{ua},
		"Accept":          []string{"*/*"},
		"Accept-Encoding": []string{"identity"},
		"Connection":      []string{"keep-alive"},
	}
	return &domain.DownloadInfo{
		URL:        url,
		Headers:    headers,
		Mode:       domain.DownloadProxy,
		Expiration: 8 * time.Hour,
		ForceProxy: true,
		Size:       fi.Size(),
		FileName:   fi.Name(),
	}, nil
}

func basicAuth(user, pw string) string {
	return base64.StdEncoding.EncodeToString([]byte(user + ":" + pw))
}

func (d *Driver) CreateFolder(ctx context.Context, parentID, name string) (*domain.FileItem, error) {
	c, err := d.ensureClient()
	if err != nil {
		return nil, err
	}
	folderName := strings.TrimSpace(name)
	if folderName == "" {
		return nil, domain.Errorf(domain.CodeValidation, "文件夹名称不能为空")
	}
	target := d.childPath(parentID, folderName)
	_ = ctx
	// gowebdav 的 Mkdir 会把 405（已存在）改写成 201 误判成功，故先 Stat 探测。
	if _, err := c.Stat(target); err == nil {
		return nil, domain.Errorf(domain.CodeValidation, "目标目录已存在同名文件夹")
	} else if !gowebdav.IsErrNotFound(err) {
		return nil, mapError(err)
	}
	if err := c.Mkdir(target, 0o755); err != nil {
		return nil, mapError(err)
	}
	return &domain.FileItem{
		ID:     target,
		Name:   folderName,
		IsDir:  true,
		IDKind: domain.IDPath,
	}, nil
}

func (d *Driver) DeleteFiles(ctx context.Context, fileIDs []string) error {
	c, err := d.ensureClient()
	if err != nil {
		return err
	}
	for _, id := range fileIDs {
		p := d.normalizePath(id)
		if p == d.rootPath() {
			return domain.Errorf(domain.CodeValidation, "根目录不支持删除")
		}
		_ = ctx
		if err := c.RemoveAll(p); err != nil {
			return mapError(err)
		}
	}
	return nil
}

func (d *Driver) MoveFiles(ctx context.Context, fileIDs []string, targetParentID, _ string) error {
	c, err := d.ensureClient()
	if err != nil {
		return err
	}
	target := d.normalizePath(targetParentID)
	for _, id := range fileIDs {
		src := d.normalizePath(id)
		if src == d.rootPath() {
			return domain.Errorf(domain.CodeValidation, "根目录不支持移动")
		}
		dst := d.childPath(target, baseName(src))
		_ = ctx
		if err := c.Rename(src, dst, true); err != nil {
			return mapError(err)
		}
	}
	return nil
}

func (d *Driver) CopyFiles(ctx context.Context, fileIDs []string, targetParentID string) error {
	c, err := d.ensureClient()
	if err != nil {
		return err
	}
	target := d.normalizePath(targetParentID)
	for _, id := range fileIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		src := d.normalizePath(id)
		if src == d.rootPath() {
			return domain.Errorf(domain.CodeValidation, "根目录不支持复制")
		}
		dst := d.childPath(target, baseName(src))
		if err := d.copyOne(ctx, c, src, dst); err != nil {
			return mapError(err)
		}
	}
	return nil
}

func (d *Driver) copyOne(ctx context.Context, c *gowebdav.Client, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := c.Copy(src, dst, true); err == nil {
		return nil
	} else if !shouldFallbackCopy(err) {
		return err
	}
	fi, err := c.Stat(src)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return d.copyDirByStream(ctx, c, src, dst)
	}
	return d.copyFileByStream(ctx, c, src, dst, fi.Size())
}

func (d *Driver) copyFileByStream(ctx context.Context, c *gowebdav.Client, src, dst string, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stream, err := c.ReadStream(src)
	if err != nil {
		return err
	}
	defer stream.Close()
	reader := &ctxReader{ctx: ctx, r: stream}
	if err := c.WriteStreamWithLength(dst, reader, size, 0o644); err != nil {
		if err == io.ErrUnexpectedEOF {
			return domain.Errorf(domain.CodeDriverError, "WebDAV 复制中断：%s", src)
		}
		return err
	}
	return nil
}

func (d *Driver) copyDirByStream(ctx context.Context, c *gowebdav.Client, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := d.ensureRemoteDir(c, dst); err != nil {
		return err
	}
	children, err := c.ReadDir(src)
	if err != nil {
		return err
	}
	for _, child := range children {
		childSrc, ok := webdavPath(child)
		if !ok {
			childSrc = d.childPath(src, child.Name())
		}
		childDst := d.childPath(dst, child.Name())
		if err := ctx.Err(); err != nil {
			return err
		}
		if child.IsDir() {
			if err := d.copyDirByStream(ctx, c, childSrc, childDst); err != nil {
				return err
			}
			continue
		}
		if err := d.copyFileByStream(ctx, c, childSrc, childDst, child.Size()); err != nil {
			return err
		}
	}
	return nil
}

func shouldFallbackCopy(err error) bool {
	return gowebdav.IsErrCode(err, http.StatusMethodNotAllowed) || gowebdav.IsErrCode(err, http.StatusNotImplemented)
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *ctxReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if err != nil {
		return n, err
	}
	if err := r.ctx.Err(); err != nil {
		if n > 0 {
			return n, nil
		}
		return 0, err
	}
	return n, nil
}

func (d *Driver) ensureRemoteDir(c *gowebdav.Client, path string) error {
	if _, err := c.Stat(path); err == nil {
		return nil
	} else if !gowebdav.IsErrNotFound(err) {
		return err
	}
	return c.MkdirAll(path, 0o755)
}

func (d *Driver) RenameFile(ctx context.Context, fileID, newName string) error {
	src := d.normalizePath(fileID)
	name := strings.TrimSpace(newName)
	if src == d.rootPath() {
		return domain.Errorf(domain.CodeValidation, "根目录不支持重命名")
	}
	if name == "" {
		return domain.Errorf(domain.CodeValidation, "新名称不能为空")
	}
	dst := d.childPath(parentPath(src), name)
	c, err := d.ensureClient()
	if err != nil {
		return err
	}
	_ = ctx
	if err := c.Rename(src, dst, true); err != nil {
		return mapError(err)
	}
	return nil
}
