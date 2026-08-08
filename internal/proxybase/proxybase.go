// Package proxybase 提供 Emby / 飞牛影视反代共用的只读辅助：
// STRM play URL 解析、URL/端口规范化、hop-by-hop 头集合与超时常量。
package proxybase

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"litepan/internal/domain"
	"litepan/internal/strm"
)

// StrmPlayPathRE 匹配 LitePan STRM play URL（与 internal/strm 播放链接格式一致）。
var StrmPlayPathRE = regexp.MustCompile(`(?i)^/api/strm/play/(\d+)/([^/]+)/t/([^/]+)/n/([^/?#\s]+)(?:/s/([^/?#\s]+))?$`)

// HopByHopHeaderNames 是反向代理转发时需剥离的 hop-by-hop 头。
var HopByHopHeaderNames = map[string]struct{}{
	"connection": {}, "keep-alive": {}, "proxy-authenticate": {}, "proxy-authorization": {},
	"te": {}, "trailers": {}, "transfer-encoding": {}, "upgrade": {}, "host": {},
}

// TestRequestTimeout 是反代连通性测试的上游请求超时。
const TestRequestTimeout = 20 * time.Second

// ParseLitePanSTRMURL 从 STRM play URL 解析账号 ID 与网盘 file_id。
func ParseLitePanSTRMURL(value string) (int64, string, bool) {
	path := LitePanPath(value)
	m := StrmPlayPathRE.FindStringSubmatch(path)
	if len(m) < 3 {
		return 0, "", false
	}
	accountID, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || accountID <= 0 {
		return 0, "", false
	}
	fileID, err := strm.DecodeFileKey(m[2])
	if err != nil || fileID == "" {
		return 0, "", false
	}
	return accountID, fileID, true
}

// LitePanPath 从 STRM 播放地址中提取路径部分（去掉 host 与 query）。
func LitePanPath(value string) string {
	text := CleanWrappedURL(value)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
		u, err := url.Parse(text)
		if err != nil {
			return ""
		}
		return u.EscapedPath()
	}
	pathOnly, _, _ := strings.Cut(text, "?")
	return pathOnly
}

// CleanWrappedURL 去掉字符串外层包裹字符（引号/反引号/空格）。
func CleanWrappedURL(value string) string {
	text := strings.TrimSpace(value)
	for {
		trimmed := strings.Trim(text, "`\"' ")
		if trimmed == text {
			return trimmed
		}
		text = strings.TrimSpace(trimmed)
	}
}

// NormalizeOptionalPort 校验并规范化可选端口号，空值返回空。
func NormalizeOptionalPort(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		return "", domain.Errorf(domain.CodeValidation, "反代端口必须是 1-65535")
	}
	return strconv.Itoa(n), nil
}

// PublicBase 根据请求头与反代端口构造对外 base URL。
func PublicBase(r *http.Request, port string) string {
	if r == nil {
		if port == "" {
			return ""
		}
		return "http://127.0.0.1:" + port
	}
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if port != "" {
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = net.JoinHostPort(h, port)
		} else {
			host = net.JoinHostPort(strings.Split(host, ":")[0], port)
		}
	}
	return scheme + "://" + host
}
