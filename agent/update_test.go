package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDownloadReleaseManifest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/release.json" {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte(`{
          "version":"0.12.0",
          "artifacts":[
            {"os":"darwin","arch":"amd64","name":"rc-darwin-amd64","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
            {"os":"darwin","arch":"arm64","name":"rc-darwin-arm64","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
            {"os":"linux","arch":"amd64","name":"rc-linux-amd64","sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
            {"os":"linux","arch":"arm64","name":"rc-linux-arm64","sha256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}
          ]
        }`))
	}))
	defer server.Close()

	previous := releaseBaseURL
	releaseBaseURL = server.URL + "/"
	t.Cleanup(func() { releaseBaseURL = previous })

	manifest, err := downloadReleaseManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "0.12.0" {
		t.Fatalf("unexpected manifest version %s", manifest.Version)
	}
}
