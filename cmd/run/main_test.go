package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/snippets-run/runners/internal/registry"
)

func TestParseIdentifier(t *testing.T) {
	owner, repo, ref, err := parseIdentifier("acme/example.sh@release/v1")
	if err != nil {
		t.Fatal(err)
	}
	if owner != "acme" || repo != "example.sh" || ref != "release/v1" {
		t.Fatalf("got %q, %q, %q", owner, repo, ref)
	}
}

func TestParseIdentifierDefaultsRefToLatest(t *testing.T) {
	owner, repo, ref, err := parseIdentifier("acme/example.sh")
	if err != nil {
		t.Fatal(err)
	}
	if owner != "acme" || repo != "example.sh" || ref != "latest" {
		t.Fatalf("got %q, %q, %q", owner, repo, ref)
	}
}

func TestParseIdentifierRejectsUnsafePath(t *testing.T) {
	if _, _, _, err := parseIdentifier("../repo@main"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseInvocationInputs(t *testing.T) {
	call, err := parseInvocation([]string{"acme/example.sh@v1", "--name=Alice", "--count=2"})
	if err != nil {
		t.Fatal(err)
	}
	if call.inputs["NAME"] != "Alice" || call.inputs["COUNT"] != "2" {
		t.Fatalf("unexpected inputs: %#v", call.inputs)
	}
}

func TestParseInvocationAliasesCreate(t *testing.T) {
	call, err := parseInvocation([]string{"create", "--name=example", "--type=sh"})
	if err != nil {
		t.Fatal(err)
	}
	if call.owner != "snippets" || call.repo != "create.sh" || call.ref != "latest" {
		t.Fatalf("unexpected alias target: %#v", call)
	}
	if call.inputs["NAME"] != "example" || call.inputs["TYPE"] != "sh" {
		t.Fatalf("unexpected inputs: %#v", call.inputs)
	}
}

func TestParseInvocationRejectsBareArguments(t *testing.T) {
	if _, err := parseInvocation([]string{"acme/example.sh@v1", "value"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseInvocationRequiresTypeSuffix(t *testing.T) {
	if _, err := parseInvocation([]string{"acme/example@v1"}); err == nil {
		t.Fatal("expected missing suffix error")
	}
}

func TestEnvironmentOverridesExistingInput(t *testing.T) {
	t.Setenv("INPUTS_NAME", "old")
	values := environment(map[string]string{"NAME": "new"})
	for _, value := range values {
		if value == "INPUTS_NAME=old" {
			t.Fatal("old input value was retained")
		}
		if value == "INPUTS_NAME=new" {
			return
		}
	}
	t.Fatal("new input value was not added")
}

func TestPrepareDownloadsAndAtomicallyCachesSnippet(t *testing.T) {
	archive := tarGz(t, "main.sh", "echo hello")
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/download/acme/example.sh@a1b2c3d4" {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Content-Type", "application/gzip")
		_, _ = response.Write(archive)
	}))
	defer server.Close()
	client, err := registry.New(server.URL, "test")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "acme", "example.sh", "a1b2c3d4")
	call := invocation{owner: "acme", repo: "example.sh"}
	if err := prepare(context.Background(), client, call, "a1b2c3d4", dir); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(dir, "main.sh"))
	if err != nil || string(contents) != "echo hello" {
		t.Fatalf("got %q, %v", contents, err)
	}
}

func tarGz(t *testing.T, name, contents string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(contents))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestValidCommit(t *testing.T) {
	if !validCommit("a1b2c3d4") {
		t.Fatal("valid commit rejected")
	}
	if validCommit("../../etc") {
		t.Fatal("unsafe commit accepted")
	}
}
