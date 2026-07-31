package webdav

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"litepan/internal/domain"
	"litepan/internal/driver"
)

func TestResolveDownloadRedirectSuccess(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if r.Header.Get("Range") == "bytes=0-0" {
				w.Header().Set("Content-Range", "bytes 0-0/10")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte("x"))
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer final.Close()

	webdavSrv := newTestWebDAVServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !hasBasicAuth(r, "user", "pass") {
			w.Header().Set("WWW-Authenticate", `Basic realm="dav"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case "PROPFIND":
			writeTestPropfind(w, r.URL.Path, "file.mkv", 10)
		case http.MethodHead, http.MethodGet:
			http.Redirect(w, r, final.URL+"/cdn/file.mkv", http.StatusFound)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	defer webdavSrv.Close()

	d := &Driver{}
	d.add = Addition{
		Address:      webdavSrv.URL + "/dav",
		Username:     "user",
		Password:     "pass",
		DownloadMode: "redirect",
	}
	if err := d.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	info, err := d.ResolveDownload(context.Background(), driver.DownloadRequest{FileID: "/file.mkv"})
	if err != nil {
		t.Fatalf("ResolveDownload: %v", err)
	}
	if info.Mode != domain.DownloadRedirect {
		t.Fatalf("Mode = %v, want redirect", info.Mode)
	}
	if info.ForceProxy {
		t.Fatalf("ForceProxy = true, want false")
	}
	if got := info.URL; got != final.URL+"/cdn/file.mkv" {
		t.Fatalf("URL = %q, want %q", got, final.URL+"/cdn/file.mkv")
	}
	if len(info.Headers) != 0 {
		t.Fatalf("Headers = %v, want empty", info.Headers)
	}
}

func TestResolveDownloadRedirectFallsBackToProxyWhenAnonymousUnavailable(t *testing.T) {
	protected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer protected.Close()

	webdavSrv := newTestWebDAVServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !hasBasicAuth(r, "user", "pass") {
			w.Header().Set("WWW-Authenticate", `Basic realm="dav"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case "PROPFIND":
			writeTestPropfind(w, r.URL.Path, "file.mkv", 10)
		case http.MethodHead, http.MethodGet:
			http.Redirect(w, r, protected.URL+"/blocked/file.mkv", http.StatusFound)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	defer webdavSrv.Close()

	d := &Driver{}
	d.add = Addition{
		Address:      webdavSrv.URL + "/dav",
		Username:     "user",
		Password:     "pass",
		DownloadMode: "redirect",
	}
	if err := d.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	info, err := d.ResolveDownload(context.Background(), driver.DownloadRequest{FileID: "/file.mkv"})
	if err != nil {
		t.Fatalf("ResolveDownload: %v", err)
	}
	if info.Mode != domain.DownloadProxy {
		t.Fatalf("Mode = %v, want proxy", info.Mode)
	}
	if !info.ForceProxy {
		t.Fatalf("ForceProxy = false, want true")
	}
	if got := info.URL; got != webdavSrv.URL+"/dav/file.mkv" {
		t.Fatalf("URL = %q, want %q", got, webdavSrv.URL+"/dav/file.mkv")
	}
	if auth := info.Headers.Get("Authorization"); auth == "" {
		t.Fatalf("Authorization header missing")
	}
}

func TestResolveDownloadRedirectFallsBackToGetWhenHeadUnavailable(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusMethodNotAllowed)
		case http.MethodGet:
			if r.Header.Get("Range") != "bytes=0-0" {
				t.Fatalf("Range = %q, want bytes=0-0", r.Header.Get("Range"))
			}
			w.Header().Set("Content-Range", "bytes 0-0/10")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("x"))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer final.Close()

	webdavSrv := newTestWebDAVServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !hasBasicAuth(r, "user", "pass") {
			w.Header().Set("WWW-Authenticate", `Basic realm="dav"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case "PROPFIND":
			writeTestPropfind(w, r.URL.Path, "file.mkv", 10)
		case http.MethodHead:
			w.WriteHeader(http.StatusMethodNotAllowed)
		case http.MethodGet:
			if r.Header.Get("Range") != "bytes=0-0" {
				t.Fatalf("Range = %q, want bytes=0-0", r.Header.Get("Range"))
			}
			http.Redirect(w, r, final.URL+"/cdn/file.mkv", http.StatusFound)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	defer webdavSrv.Close()

	d := &Driver{}
	d.add = Addition{
		Address:      webdavSrv.URL + "/dav",
		Username:     "user",
		Password:     "pass",
		DownloadMode: "redirect",
	}
	if err := d.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}

	info, err := d.ResolveDownload(context.Background(), driver.DownloadRequest{FileID: "/file.mkv"})
	if err != nil {
		t.Fatalf("ResolveDownload: %v", err)
	}
	if info.Mode != domain.DownloadRedirect {
		t.Fatalf("Mode = %v, want redirect", info.Mode)
	}
	if got := info.URL; got != final.URL+"/cdn/file.mkv" {
		t.Fatalf("URL = %q, want %q", got, final.URL+"/cdn/file.mkv")
	}
}

func newTestWebDAVServer(t *testing.T, next http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/dav") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		next(w, r)
	}))
}

func hasBasicAuth(r *http.Request, wantUser, wantPass string) bool {
	user, pass, ok := r.BasicAuth()
	return ok && user == wantUser && pass == wantPass
}

func writeTestPropfind(w http.ResponseWriter, href, name string, size int64) {
	w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:">
  <d:response>
    <d:href>%s</d:href>
    <d:propstat>
      <d:prop>
        <d:displayname>%s</d:displayname>
        <d:getcontentlength>%d</d:getcontentlength>
        <d:getcontenttype>video/mp4</d:getcontenttype>
        <d:getlastmodified>Mon, 02 Jan 2006 15:04:05 GMT</d:getlastmodified>
      </d:prop>
      <d:status>HTTP/1.1 200 OK</d:status>
    </d:propstat>
  </d:response>
</d:multistatus>`, href, name, size)
}
