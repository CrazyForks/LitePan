package upload

import (
	"strings"
	"time"

	"litepan/pkg/speedsmoother"
)

func (m *Manager) updateProgress(taskID string, uploaded, total int64, message string) {
	if total <= 0 {
		total = 1
	}
	progress := calcProgress(uploaded, total)
	if message == "" {
		message = "正在上传到网盘"
	}
	now := time.Now()

	m.mu.Lock()
	st, ok := m.tasks[taskID]
	if !ok {
		m.mu.Unlock()
		return
	}
	prevSpeed := st.SpeedBytesPerSecond
	speedSample := st.speed.Sample(uploaded, now, speedsmoother.PhaseKey(message))
	st.SpeedBytesPerSecond = speedSample.Display

	emit := shouldEmitProgress(st, progress, uploaded, total, message, now)
	if !emit {
		m.mu.Unlock()
		if st.SpeedBytesPerSecond != prevSpeed {
			m.broadcast()
		}
		return
	}
	if uploaded < st.UploadedBytes && st.UploadedBytes > 0 {
		uploaded = st.UploadedBytes
		progress = calcProgress(uploaded, total)
	}
	st.lastEmit = now
	st.lastProgress = progress
	st.lastMessage = message
	st.Status = StatusRunning
	st.Progress = progress
	st.UploadedBytes = uploaded
	st.TotalBytes = total
	st.Message = message
	st.UpdatedAt = unixFloat(now)
	m.mu.Unlock()
	m.broadcast()
}

func calcProgress(uploaded, total int64) int {
	if total <= 0 {
		return 0
	}
	progress := int(uploaded * 100 / total)
	if uploaded >= total {
		return 100
	}
	if progress > 99 {
		return 99
	}
	return progress
}

func shouldEmitProgress(st *taskState, progress int, uploaded, total int64, message string, now time.Time) bool {
	if st.lastEmit.IsZero() || uploaded >= total || message != st.lastMessage {
		return true
	}
	if uploaded > st.UploadedBytes && now.Sub(st.lastEmit) >= progressInterval {
		return true
	}
	return progress != st.lastProgress &&
		(progress >= st.lastProgress+1 || now.Sub(st.lastEmit) >= progressInterval)
}

func translateError(msg string) string {
	switch {
	case strings.Contains(msg, "Server disconnected"):
		return "服务器连接已断开"
	case strings.Contains(msg, "Connection timeout"), strings.Contains(msg, "Timeout"):
		return "连接服务器超时"
	case strings.Contains(msg, "Network Error"), strings.Contains(msg, "Failed to fetch"):
		return "网络连接异常"
	default:
		return msg
	}
}

func uploadEntryName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[i+1:]
	}
	return name
}
