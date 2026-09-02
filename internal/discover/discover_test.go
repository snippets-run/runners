package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindNodeEntrypoint(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"package.json", "index.js", "main.js"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entrypoint, err := Find(dir)
	if err != nil {
		t.Fatal(err)
	}
	if entrypoint.Runtime != Node || entrypoint.File != "index.js" {
		t.Fatalf("unexpected entrypoint: %#v", entrypoint)
	}
}

func TestFindRejectsAmbiguousRuntime(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"main.py", "run.sh"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Find(dir); err == nil {
		t.Fatal("expected ambiguity error")
	}
}
