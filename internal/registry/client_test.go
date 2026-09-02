package registry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveAndDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/resolve/acme/example@v1":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"owner":"acme","repo":"example","ref":"v1","commit":"a1b2c3d4"}`))
		case "/api/download/acme/example@a1b2c3d4":
			response.Header().Set("Content-Type", "application/gzip")
			_, _ = response.Write([]byte("archive"))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client, err := New(server.URL, "test")
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := client.Resolve(context.Background(), "acme", "example", "v1")
	if err != nil || resolution.Commit != "a1b2c3d4" {
		t.Fatalf("got %#v, %v", resolution, err)
	}
	archive, err := client.Download(context.Background(), "acme", "example", resolution.Commit)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	contents, err := io.ReadAll(archive)
	if err != nil || string(contents) != "archive" {
		t.Fatalf("got %q, %v", contents, err)
	}
}

func TestResolveUsesJSONErrorMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNotFound)
		_, _ = response.Write([]byte(`{"error":"snippet not found"}`))
	}))
	defer server.Close()

	client, err := New(server.URL, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Resolve(context.Background(), "acme", "missing", "v1")
	if err == nil || err.Error() != "registry returned 404: snippet not found" {
		t.Fatalf("unexpected error: %v", err)
	}
}
