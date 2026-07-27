package upload

import (
	"context"

	"litepan/internal/core/driverexec"
	"litepan/internal/driver"
	"litepan/internal/eventbus"
)

func (m *Manager) executeUpload(ctx context.Context, taskID string) {
	m.mu.Lock()
	st, ok := m.tasks[taskID]
	if !ok {
		m.mu.Unlock()
		return
	}
	resume := cloneMap(st.resumeData)
	resuming := len(resume) > 0
	progress, uploaded := resumedProgress(st)
	accountID := st.AccountID
	localPath := st.localPath
	fileName := st.FileName
	targetPath := st.TargetPath
	conflictPolicy := st.conflictPolicy
	m.mu.Unlock()

	msg := "正在上传到网盘"
	if resuming {
		msg = "正在继续上传到网盘"
	}
	started := false
	m.patch(taskID, func(st *taskState) {
		if ctx.Err() != nil || st.Status != StatusPending {
			return
		}
		started = true
		st.Status = StatusRunning
		st.Progress = progress
		st.UploadedBytes = uploaded
		st.SpeedBytesPerSecond = 0
		st.Message = msg
		st.Error = ""
		st.speed.Reset()
	})
	if !started {
		return
	}

	entryName := uploadEntryName(fileName)

	result, err := m.runLocalUpload(ctx, accountID, driver.LocalUploadRequest{
		LocalPath:      localPath,
		FileName:       entryName,
		ParentID:       targetPath,
		ConflictPolicy: conflictPolicy,
		ResumeState:    resume,
		OnResumeState: func(state map[string]any) {
			m.applyResumeState(taskID, state)
		},
		OnProgress: func(uploaded, total int64, message string) {
			m.updateProgress(taskID, uploaded, total, message)
		},
	})

	m.mu.Lock()
	st, ok = m.tasks[taskID]
	if !ok {
		m.mu.Unlock()
		return
	}
	mode := st.cancelMode
	m.mu.Unlock()

	if err != nil {
		if ctx.Err() != nil {
			if mode == "pause" {
				m.patch(taskID, func(st *taskState) {
					st.Status = StatusPaused
					st.SpeedBytesPerSecond = 0
					st.Message = "上传已暂停"
				})
				return
			}
			m.patch(taskID, func(st *taskState) {
				st.Status = StatusCanceled
				st.SpeedBytesPerSecond = 0
				st.Message = "上传任务已取消"
				st.Error = "上传任务已取消"
			})
			return
		}
		m.failTask(taskID, err.Error())
		return
	}

	status := StatusSuccess
	msg = result.Message
	if result.Skipped {
		status = StatusSkipped
	}
	m.patch(taskID, func(st *taskState) {
		st.Status = status
		st.Progress = 100
		st.UploadedBytes = st.TotalBytes
		st.SpeedBytesPerSecond = 0
		st.Message = msg
		st.Error = ""
		st.resumeData = nil
		st.Result = map[string]any{
			"file_id":   result.FileID,
			"parent_id": result.ParentID,
			"file_name": result.FileName,
			"size":      result.Size,
		}
	})
	m.removeLocalFile(localPath)
	if m.files == nil && m.bus != nil {
		parentID := result.ParentID
		if parentID == "" {
			parentID = targetPath
		}
		m.bus.Publish(context.Background(), eventbus.FileMutated{
			AccountID: accountID,
			Op:        "upload",
			ParentID:  parentID,
			FileID:    result.FileID,
		})
	}
}

func (m *Manager) deleteUploadedFile(ctx context.Context, st *taskState) error {
	if st.Result == nil {
		return nil
	}
	raw, _ := st.Result["file_id"].(string)
	if raw == "" {
		return nil
	}
	if err := m.exec.Check(ctx, st.AccountID); err != nil {
		return err
	}
	err := m.exec.Run(ctx, st.AccountID, func(drv driver.Driver) error {
		deleter, err := driverexec.Require[driver.Deleter](drv)
		if err != nil {
			return err
		}
		return deleter.DeleteFiles(ctx, []string{raw})
	})
	if err != nil {
		return err
	}
	m.publishUploadedFileDeleted(st, raw)
	return nil
}

func (m *Manager) publishUploadedFileDeleted(st *taskState, fileID string) {
	if m.bus == nil {
		return
	}
	parentID, _ := st.Result["parent_id"].(string)
	if parentID == "" {
		parentID = st.TargetPath
	}
	m.bus.Publish(context.Background(), eventbus.FileMutated{
		AccountID: st.AccountID,
		Op:        "delete",
		ParentID:  parentID,
		FileIDs:   []string{fileID},
	})
}
