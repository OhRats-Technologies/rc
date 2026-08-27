package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReplaceExecutableSkipsCurrentVersion(t *testing.T) {
	artifactRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/release.json" {
			artifactRequests++
			http.Error(writer, "artifact should not be requested", http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(writer, `{
          "version":%q,
          "artifacts":[
            {"os":"darwin","arch":"amd64","name":"rc-darwin-amd64","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
            {"os":"darwin","arch":"arm64","name":"rc-darwin-arm64","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
            {"os":"linux","arch":"amd64","name":"rc-linux-amd64","sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
            {"os":"linux","arch":"arm64","name":"rc-linux-arm64","sha256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}
          ]
        }`, version)
	}))
	defer server.Close()

	previousRelease := releaseBaseURL
	previousTagged := taggedReleaseBaseURL
	releaseBaseURL = server.URL + "/"
	taggedReleaseBaseURL = server.URL + "/"
	t.Cleanup(func() {
		releaseBaseURL = previousRelease
		taggedReleaseBaseURL = previousTagged
	})

	updated, err := replaceExecutable()
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("current version was reported as updated")
	}
	if artifactRequests != 0 {
		t.Fatalf("current version downloaded %d artifact(s)", artifactRequests)
	}
}
