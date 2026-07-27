package upload

import "os"

func (m *Manager) removeLocalFile(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}
