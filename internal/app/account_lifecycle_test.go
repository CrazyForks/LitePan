package app

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"litepan/internal/favorites"
)

func TestAccountLifecycleDeleteRemovesFavorites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	favoritesSvc := favorites.NewService(filepath.Join(dir, "litepan.db"), logger)
	ctx := context.Background()
	for _, accountID := range []int64{11, 22} {
		if _, err := favoritesSvc.Put(ctx, accountID, favorites.AccountState{
			Items: []favorites.Item{{
				ID:   "folder",
				Name: "收藏目录",
				Crumbs: []favorites.Crumb{{
					ID:   "root",
					Name: "根目录",
				}},
			}},
		}); err != nil {
			t.Fatalf("保存账号 %d 收藏失败: %v", accountID, err)
		}
	}

	lifecycle := accountLifecycle{favorites: favoritesSvc}
	if err := lifecycle.OnAccountDeleted(ctx, 11); err != nil {
		t.Fatalf("执行账号删除生命周期失败: %v", err)
	}
	deleted, err := favoritesSvc.Get(ctx, 11)
	if err != nil {
		t.Fatalf("读取目标账号收藏失败: %v", err)
	}
	if len(deleted.Items) != 0 {
		t.Fatalf("账号删除生命周期未清理收藏: %#v", deleted)
	}
	kept, err := favoritesSvc.Get(ctx, 22)
	if err != nil {
		t.Fatalf("读取其他账号收藏失败: %v", err)
	}
	if len(kept.Items) != 1 {
		t.Fatalf("其他账号收藏被误清理: %#v", kept)
	}
}
