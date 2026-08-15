package domain

import (
	"context"
	"time"
)

// QuarkTVBinding 是夸克 TV 播放接管插件绑定的 TV 凭证，与某个夸克网盘账号一一对应。
type QuarkTVBinding struct {
	AccountID      int64
	DeviceID       string
	RefreshToken   string
	AccessToken    string
	TokenExpiresAt time.Time
	TVUID          string
	TVNickname     string
	BoundAt        time.Time
}

// QuarkTVBindingRepository 维护夸克 TV 绑定的持久化记录。
type QuarkTVBindingRepository interface {
	Get(ctx context.Context, accountID int64) (*QuarkTVBinding, error)
	Upsert(ctx context.Context, b *QuarkTVBinding) error
	Delete(ctx context.Context, accountID int64) error
}
