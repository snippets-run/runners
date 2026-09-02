package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatusAndClean(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "owner", "repo", "commit", "main.sh")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("echo hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	size, err := Status(root)
	if err != nil || size != int64(len("echo hello")) {
		t.Fatalf("got %d, %v", size, err)
	}
	if err := Clean(root); err != nil {
		t.Fatal(err)
	}
	if Ready(root) {
		t.Fatal("cache directory remains after clean")
	}
}

func TestCleanRefusesWorkingDirectory(t *testing.T) {
	if err := Clean("."); err == nil {
		t.Fatal("expected unsafe cache path error")
	}
}
