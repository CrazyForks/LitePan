package guangya

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"litepan/internal/driver"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testJSONResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestRapidUploadByHashUsesOfficialTwoStepFlow(t *testing.T) {
	var calls []string
	d := &Driver{
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls = append(calls, req.URL.Path)
				switch req.URL.Path {
				case pathUploadToken:
					return testJSONResponse(`{"code":0,"msg":"ok","data":{"taskId":"task-1"}}`), nil
				case pathCheckFlashUpload:
					return testJSONResponse(`{"code":0,"msg":"ok","data":{"canFlashUpload":true}}`), nil
				case pathUploadTaskInfo:
					return testJSONResponse(`{"code":0,"msg":"ok","data":{"fileId":"file-1"}}`), nil
				default:
					t.Fatalf("unexpected path: %s", req.URL.Path)
					return nil, nil
				}
			}),
		},
		deviceIDVal: strings.Repeat("a", 32),
		token:       "test-token",
	}

	result, err := d.RapidUploadByHash(context.Background(), driver.RapidUploadRequest{
		ParentID: "folder-1",
		FileName: "video.mkv",
		Method:   "md5",
		Hash:     "0123456789abcdef0123456789abcdef",
		Size:     123,
	})
	if err != nil {
		t.Fatalf("RapidUploadByHash error = %v", err)
	}
	if result == nil || !result.Reuse || result.FileID != "file-1" {
		t.Fatalf("RapidUploadByHash result = %#v", result)
	}
	got := strings.Join(calls, " -> ")
	want := strings.Join([]string{
		pathUploadToken,
		pathCheckFlashUpload,
		pathUploadTaskInfo,
	}, " -> ")
	if got != want {
		t.Fatalf("request path order = %s, want %s", got, want)
	}
}
