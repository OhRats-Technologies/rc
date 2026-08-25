package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
)

func TestDownloadSignedManifestRetriesMismatchedPair(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	releaseDir := filepath.Join(filepath.Dir(file), "..", "release")
	manifest, err := os.ReadFile(filepath.Join(releaseDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	signature, err := os.ReadFile(filepath.Join(releaseDir, "manifest.sig"))
	if err != nil {
		t.Fatal(err)
	}
	var signatureRequests atomic.Int32
	var sawCacheBuster atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("pair") != "" {
			sawCacheBuster.Store(true)
		}
		switch request.URL.Path {
		case "/release.json":
			_, _ = writer.Write(manifest)
		case "/release.json.sig":
			if signatureRequests.Add(1) == 1 {
				_, _ = writer.Write([]byte("rolling-deploy-mismatch"))
				return
			}
			_, _ = writer.Write(signature)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	value, err := downloadSignedManifest(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if value.Version != version {
		t.Fatalf("unexpected manifest version %s", value.Version)
	}
	if signatureRequests.Load() < 2 {
		t.Fatal("release pair was not retried after signature mismatch")
	}
	if !sawCacheBuster.Load() {
		t.Fatal("release pair requests did not bypass intermediary caches")
	}
}
