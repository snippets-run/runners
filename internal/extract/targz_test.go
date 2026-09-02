package extract

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func archive(t *testing.T, name, contents string) *bytes.Reader {
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tarWriter := tar.NewWriter(gz)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(contents))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(data.Bytes())
}

func TestTarGzExtractsFiles(t *testing.T) {
	dir := t.TempDir()
	if err := TarGz(archive(t, "main.sh", "echo hello"), dir); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(dir, "main.sh"))
	if err != nil || string(contents) != "echo hello" {
		t.Fatalf("got %q, %v", contents, err)
	}
}

func TestTarGzRejectsTraversal(t *testing.T) {
	if err := TarGz(archive(t, "../outside", "bad"), t.TempDir()); err == nil {
		t.Fatal("expected path traversal error")
	}
}
