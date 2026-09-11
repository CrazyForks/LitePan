package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"litepan/internal/notification"
	"litepan/internal/store"
)

// sseRecorder 给 recorder 的写入加锁：handler 在自己的 goroutine 里持续写，
// 测试线程需要随时读取已落盘的事件内容。
type sseRecorder struct {
	*httptest.ResponseRecorder
	mu sync.Mutex
}

func (r *sseRecorder) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ResponseRecorder.Write(b)
}

func (r *sseRecorder) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ResponseRecorder.Flush()
}

func (r *sseRecorder) body() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ResponseRecorder.Body.String()
}

func TestStreamNotificationUnreadPushesCount(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, store.Options{Memory: true})
	if err != nil {
		t.Fatalf("打开内存库: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("迁移: %v", err)
	}
	svc := notification.NewService(notification.Options{Repo: store.New(db).Notifications})
	handler := &Handler{notifications: svc}

	reqCtx, cancel := context.WithCancel(ctx)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/notifications/stream", nil).WithContext(reqCtx)
	rec := &sseRecorder{ResponseRecorder: httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.streamNotificationUnread(rec, req)
	}()

	waitBody := func(want string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(rec.body(), want) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("未等到 %q，当前响应：%q", want, rec.body())
	}

	// 首个事件是当前未读数，前端一连上就能拿到权威值。
	waitBody("event: unread")
	waitBody(`{"count":0}`)

	// 新通知落库后应主动推送，不需要前端轮询。
	svc.Notify(ctx, "info", "system", "标题", "内容", 0, 0)
	waitBody(`{"count":1}`)

	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q", got)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("请求上下文取消后 SSE handler 未退出")
	}
}
