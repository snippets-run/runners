package cache

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func SnippetDir(root, owner, repo, commit string) string {
	return filepath.Join(root, owner, repo, commit)
}

func Ready(dir string) bool {
	entries, err := os.ReadDir(dir)
	return err == nil && len(entries) > 0
}

func EnsureParent(dir string) error {
	return os.MkdirAll(filepath.Dir(dir), 0o755)
}

func Status(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return total, err
}

func Clean(root string) error {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	if root == "" || absolute == string(filepath.Separator) || absolute == workingDirectory {
		return fmt.Errorf("refusing to clean unsafe cache path %q", root)
	}
	return os.RemoveAll(absolute)
}
