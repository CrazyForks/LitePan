package upload

import "context"

func (m *Manager) HideRelayTask(_ context.Context, taskID string) (*Task, bool) {
	var out *Task
	found := false
	m.patch(taskID, func(st *taskState) {
		found = true
		st.RelayVisible = false
		out = snapshotCopy(st)
	})
	return out, found
}

func (m *Manager) HideRelayTasks(ctx context.Context, taskIDs []string) []string {
	hidden := make([]string, 0, len(taskIDs))
	for _, id := range taskIDs {
		if _, ok := m.HideRelayTask(ctx, id); ok {
			hidden = append(hidden, id)
		}
	}
	return hidden
}
