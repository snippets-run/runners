package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindNodeEntrypoint(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"package.json", "index.js"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entrypoint, err := Find(dir, "hello.js")
	if err != nil {
		t.Fatal(err)
	}
	if entrypoint.Runtime != Node || entrypoint.File != "index.js" {
		t.Fatalf("unexpected entrypoint: %#v", entrypoint)
	}
}

func TestFindUsesSingleEntrypoint(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"main.py", "main.sh"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entrypoint, err := Find(dir, "hello.sh")
	if err != nil {
		t.Fatal(err)
	}
	if entrypoint.File != "main.sh" {
		t.Fatalf("unexpected entrypoint: %#v", entrypoint)
	}
}

func TestFindRejectsFormerFallbackEntrypoint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(dir, "hello.sh"); err == nil {
		t.Fatal("expected missing main.sh error")
	}
}

func TestFromRepository(t *testing.T) {
	tests := map[string]Runtime{"hello.sh": Shell, "hello.js": Node, "hello.py": Python}
	for repo, expected := range tests {
		runtime, err := FromRepository(repo)
		if err != nil || runtime != expected {
			t.Fatalf("%s: got %q, %v", repo, runtime, err)
		}
	}
	if _, err := FromRepository("hello"); err == nil {
		t.Fatal("expected missing suffix error")
	}
}

func TestFindJSRepositoryUsesMJSModuleEntrypoint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.mjs"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	entrypoint, err := Find(dir, "hello.js")
	if err != nil {
		t.Fatal(err)
	}
	if entrypoint.Runtime != Node || entrypoint.File != "index.mjs" {
		t.Fatalf("unexpected entrypoint: %#v", entrypoint)
	}
}

func TestFindJSRejectsMultipleEntrypoints(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"index.js", "index.mjs"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Find(dir, "hello.js"); err == nil {
		t.Fatal("expected ambiguous entrypoint error")
	}
}
