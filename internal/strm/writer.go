package strm

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func MediaStem(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return name
	}
	return strings.TrimSuffix(name, ext)
}

func SafeName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "_"
	}
	repl := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	name = repl.Replace(name)
	if name == "" || name == "." || name == ".." {
		return "_"
	}
	return name
}

func LocalRelPath(outputFolder string, relDirs []string, fileName string, isoFilenameEnabled bool) string {
	parts := make([]string, 0, len(relDirs)+2)
	parts = append(parts, SafeName(outputFolder))
	for _, dir := range relDirs {
		parts = append(parts, SafeName(dir))
	}
	parts = append(parts, LocalStrmFileName(fileName, isoFilenameEnabled))
	return filepath.Join(parts...)
}

func LegacyLocalRelPath(outputFolder string, relDirs []string, fileName string) string {
	return LocalRelPath(outputFolder, relDirs, fileName, false)
}

func LocalStrmFileName(fileName string, isoFilenameEnabled bool) string {
	if isoFilenameEnabled && isISOFileName(fileName) {
		return SafeName(fileName) + ".strm"
	}
	return SafeName(MediaStem(fileName)) + ".strm"
}

func isISOFileName(fileName string) bool {
	return strings.EqualFold(filepath.Ext(strings.TrimSpace(fileName)), ".iso")
}

func MigrateLegacyISOStrmFile(root, outputFolder string, relDirs []string, fileName, fileID string, isoFilenameEnabled bool) (bool, error) {
	if !isoFilenameEnabled || !isISOFileName(fileName) {
		return false, nil
	}
	legacyRelPath := LegacyLocalRelPath(outputFolder, relDirs, fileName)
	currentRelPath := LocalRelPath(outputFolder, relDirs, fileName, true)
	if legacyRelPath == currentRelPath {
		return false, nil
	}
	legacyPath := filepath.Join(root, legacyRelPath)
	currentPath := filepath.Join(root, currentRelPath)
	if pathHasOversizedComponent(legacyPath) || pathHasOversizedComponent(currentPath) {
		return false, nil
	}
	content, err := os.ReadFile(legacyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !strmContentReferencesFile(string(content), fileID) {
		return false, nil
	}
	if _, err := os.Stat(currentPath); err == nil {
		return true, os.Remove(legacyPath)
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		return false, err
	}
	if err := os.Rename(legacyPath, currentPath); err != nil {
		return false, err
	}
	return true, nil
}

func strmContentReferencesFile(content, fileID string) bool {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return false
	}
	return strings.Contains(content, "/"+EncodeFileKey(fileID)+"/t/")
}

func TaskOutputDir(strmDir, outputFolder string) string {
	outputFolder = strings.TrimSpace(outputFolder)
	if outputFolder == "" {
		return ""
	}
	root := strings.TrimSpace(strmDir)
	if root == "" {
		root = "strm"
	}
	return filepath.Join(root, SafeName(outputFolder))
}

func DeleteTaskOutput(strmDir, outputFolder string) error {
	dir := TaskOutputDir(strmDir, outputFolder)
	if dir == "" {
		return nil
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(dir)
}

// removeStrmScrapeIndex 删除 STRM 刮削海报墙索引（与 strmscrape.TaskIndexPath 约定一致）。
func removeStrmScrapeIndex(dataDir string, taskID int64) {
	base := filepath.Join(strings.TrimSpace(dataDir), "strmscrape", strconv.FormatInt(taskID, 10)+".sqlite")
	for _, p := range []string{base, base + "-wal", base + "-shm"} {
		_ = os.Remove(p)
	}
}

func WriteStrmFile(rootDir, relPath, content string) error {
	full := filepath.Join(rootDir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(content+"\n"), 0o644)
}
