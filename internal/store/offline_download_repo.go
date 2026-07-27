package store

import (
	"context"
	"database/sql"

	"litepan/internal/domain"
)

type offlineDownloadTaskRepo struct{ db *DB }

func (r *offlineDownloadTaskRepo) Upsert(ctx context.Context, rec *domain.OfflineDownloadTaskRecord) error {
	if rec == nil || rec.TaskID == "" {
		return domain.Errorf(domain.CodeValidation, "无效离线下载任务")
	}
	_, err := r.db.write.ExecContext(ctx, `
INSERT INTO offline_download_tasks(
    task_id, account_id, account_name, driver_type, source_kind, source, name,
    provider_task_id, info_hash, target_parent_id, target_display_path, status,
    progress, size, file_id, message, error, remote_delete, created_at, updated_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(task_id) DO UPDATE SET
    account_id=excluded.account_id,
    account_name=excluded.account_name,
    driver_type=excluded.driver_type,
    source_kind=excluded.source_kind,
    source=excluded.source,
    name=excluded.name,
    provider_task_id=excluded.provider_task_id,
    info_hash=excluded.info_hash,
    target_parent_id=excluded.target_parent_id,
    target_display_path=excluded.target_display_path,
    status=excluded.status,
    progress=excluded.progress,
    size=excluded.size,
    file_id=excluded.file_id,
    message=excluded.message,
    error=excluded.error,
    remote_delete=excluded.remote_delete,
    created_at=excluded.created_at,
    updated_at=excluded.updated_at`,
		rec.TaskID, rec.AccountID, rec.AccountName, rec.DriverType, rec.SourceKind, rec.Source, rec.Name,
		rec.ProviderTaskID, rec.InfoHash, rec.TargetParentID, rec.TargetDisplayPath, rec.Status,
		rec.Progress, rec.Size, rec.FileID, rec.Message, rec.Error, rec.RemoteDelete, rec.CreatedAt, rec.UpdatedAt,
	)
	return wrapDB(err)
}

func (r *offlineDownloadTaskRepo) Delete(ctx context.Context, taskID string) error {
	_, err := r.db.write.ExecContext(ctx, `DELETE FROM offline_download_tasks WHERE task_id=?`, taskID)
	return wrapDB(err)
}

func (r *offlineDownloadTaskRepo) DeleteByAccount(ctx context.Context, accountID int64) (int64, error) {
	result, err := r.db.write.ExecContext(ctx, `DELETE FROM offline_download_tasks WHERE account_id=?`, accountID)
	if err != nil {
		return 0, wrapDB(err)
	}
	count, err := result.RowsAffected()
	return count, wrapDB(err)
}

func (r *offlineDownloadTaskRepo) List(ctx context.Context) ([]*domain.OfflineDownloadTaskRecord, error) {
	rows, err := r.db.read.QueryContext(ctx, `
SELECT task_id, account_id, account_name, driver_type, source_kind, source, name,
       provider_task_id, info_hash, target_parent_id, target_display_path, status,
       progress, size, file_id, message, error, remote_delete, created_at, updated_at
FROM offline_download_tasks ORDER BY created_at DESC`)
	if err != nil {
		return nil, wrapDB(err)
	}
	defer rows.Close()
	out := make([]*domain.OfflineDownloadTaskRecord, 0)
	for rows.Next() {
		rec, err := scanOfflineDownloadTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, wrapDB(rows.Err())
}

func scanOfflineDownloadTask(rows *sql.Rows) (*domain.OfflineDownloadTaskRecord, error) {
	var rec domain.OfflineDownloadTaskRecord
	err := rows.Scan(
		&rec.TaskID, &rec.AccountID, &rec.AccountName, &rec.DriverType, &rec.SourceKind, &rec.Source, &rec.Name,
		&rec.ProviderTaskID, &rec.InfoHash, &rec.TargetParentID, &rec.TargetDisplayPath, &rec.Status,
		&rec.Progress, &rec.Size, &rec.FileID, &rec.Message, &rec.Error, &rec.RemoteDelete, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		return nil, wrapDB(err)
	}
	return &rec, nil
}
