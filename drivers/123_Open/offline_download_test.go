package pan123open

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"litepan/internal/domain"
	"litepan/internal/driver"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestRefreshOfflineTasksKeepsMissingTaskPendingBeforeThreshold(t *testing.T) {
	drv := &Driver{
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != pathOfflineDownloadProcess {
					t.Fatalf("unexpected path: %s", req.URL.Path)
				}
				return newJSONResponse(http.StatusOK, `{"code":1,"message":"未找到任务ID","data":null}`), nil
			}),
		},
	}
	drv.SetAuthCredentials(driverdomainCreds())

	for attempt := 1; attempt < offlineTaskMissingThreshold; attempt++ {
		updates, err := drv.RefreshOfflineTasks(context.Background(), []driver.OfflineTaskRef{{
			ProviderTaskID: "task-1",
		}})
		if err != nil {
			t.Fatalf("attempt %d RefreshOfflineTasks() error = %v", attempt, err)
		}
		if len(updates) != 1 {
			t.Fatalf("attempt %d updates len = %d, want 1", attempt, len(updates))
		}
		if updates[0].Status != driver.OfflineStatusPending {
			t.Fatalf("attempt %d status = %q, want %q", attempt, updates[0].Status, driver.OfflineStatusPending)
		}
	}
}

func TestRefreshOfflineTasksMarksMissingTaskFailedAtThreshold(t *testing.T) {
	drv := &Driver{
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != pathOfflineDownloadProcess {
					t.Fatalf("unexpected path: %s", req.URL.Path)
				}
				return newJSONResponse(http.StatusOK, `{"code":1,"message":"未找到任务ID","data":null}`), nil
			}),
		},
	}
	drv.SetAuthCredentials(driverdomainCreds())

	for attempt := 1; attempt < offlineTaskMissingThreshold; attempt++ {
		if _, err := drv.RefreshOfflineTasks(context.Background(), []driver.OfflineTaskRef{{ProviderTaskID: "task-2"}}); err != nil {
			t.Fatalf("warmup attempt %d error = %v", attempt, err)
		}
	}
	updates, err := drv.RefreshOfflineTasks(context.Background(), []driver.OfflineTaskRef{{
		ProviderTaskID: "task-2",
	}})
	if err != nil {
		t.Fatalf("RefreshOfflineTasks() error = %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("updates len = %d, want 1", len(updates))
	}
	if updates[0].Status != driver.OfflineStatusFailed {
		t.Fatalf("status = %q, want %q", updates[0].Status, driver.OfflineStatusFailed)
	}
	if updates[0].Message != "任务已在 123 云盘侧移除" {
		t.Fatalf("message = %q", updates[0].Message)
	}
}

func TestRefreshOfflineTasksResetsMissingCounterAfterSuccessfulQuery(t *testing.T) {
	calls := 0
	drv := &Driver{
		client: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				switch calls {
				case 1, 3:
					return newJSONResponse(http.StatusOK, `{"code":1,"message":"未找到任务ID","data":null}`), nil
				case 2:
					return newJSONResponse(http.StatusOK, `{"code":0,"message":"ok","data":{"process":10,"status":0}}`), nil
				default:
					t.Fatalf("unexpected call count: %d", calls)
					return nil, nil
				}
			}),
		},
	}
	drv.SetAuthCredentials(driverdomainCreds())

	updates, err := drv.RefreshOfflineTasks(context.Background(), []driver.OfflineTaskRef{{ProviderTaskID: "task-3"}})
	if err != nil || len(updates) != 1 || updates[0].Status != driver.OfflineStatusPending {
		t.Fatalf("first missing result unexpected: updates=%#v err=%v", updates, err)
	}
	updates, err = drv.RefreshOfflineTasks(context.Background(), []driver.OfflineTaskRef{{ProviderTaskID: "task-3"}})
	if err != nil || len(updates) != 1 || updates[0].Status != driver.OfflineStatusRunning {
		t.Fatalf("running result unexpected: updates=%#v err=%v", updates, err)
	}
	updates, err = drv.RefreshOfflineTasks(context.Background(), []driver.OfflineTaskRef{{ProviderTaskID: "task-3"}})
	if err != nil || len(updates) != 1 || updates[0].Status != driver.OfflineStatusPending {
		t.Fatalf("counter should reset after success: updates=%#v err=%v", updates, err)
	}
}

func driverdomainCreds() domain.AuthCredentials {
	return domain.AuthCredentials{AccessToken: "test-token"}
}
