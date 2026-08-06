package cloud189

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func writeTempUploadPartFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "upload-part-*.bin")
	if err != nil {
		t.Fatalf("创建临时文件失败: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		t.Fatalf("写入临时文件失败: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("关闭临时文件失败: %v", err)
	}
	return f.Name()
}

func TestPutUploadPartRetriesTransientServerError(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = io.Copy(io.Discard, r.Body)
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>InternalError</Code><Message>temporary failure</Message></Error>`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	driver := &Driver{uploadClient: server.Client()}
	localPath := writeTempUploadPartFile(t, "hello world")
	err := driver.putUploadPart(context.Background(), server.URL+"/upload", nil, localPath, 0, 5, 13, 134, 0, 5, nil)
	if err != nil {
		t.Fatalf("期望自动重试后成功，实际报错: %v", err)
	}
	if calls != 3 {
		t.Fatalf("请求次数=%d，期望 3", calls)
	}
}

func TestPutUploadPartStopsOnNonRetryableStatus(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, "bad request")
	}))
	defer server.Close()

	driver := &Driver{uploadClient: server.Client()}
	localPath := writeTempUploadPartFile(t, "hello world")
	err := driver.putUploadPart(context.Background(), server.URL+"/upload", nil, localPath, 0, 5, 13, 134, 0, 5, nil)
	if err == nil {
		t.Fatal("期望返回错误，实际为 nil")
	}
	if calls != 1 {
		t.Fatalf("请求次数=%d，期望 1", calls)
	}
	if strings.Contains(err.Error(), "已自动重试") {
		t.Fatalf("非瞬时错误不应带自动重试提示: %v", err)
	}
}
