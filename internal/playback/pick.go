package playback

import (
	"litepan/internal/domain"
)

type Action uint8

const (
	ActionRedirect Action = iota
	ActionStream
)

type Intent struct {
	ForceProxy   bool
	FileName     string
	Inline       bool
	WebDAV       bool
	OriginalFile bool
}

// allowsPlaybackResolve 只有真实播放请求才允许增强工具替换原始下载地址。
// WebDAV 和视频海报取帧都需要原始文件字节。
func (intent Intent) allowsPlaybackResolve() bool {
	return !intent.WebDAV && !intent.OriginalFile
}

func PickAction(mode domain.DownloadMode, link domain.DownloadInfo, intent Intent) Action {
	if intent.ForceProxy || link.ForceProxy {
		return ActionStream
	}
	switch mode {
	case domain.DownloadRedirect:
		return ActionRedirect
	default:
		return ActionStream
	}
}
