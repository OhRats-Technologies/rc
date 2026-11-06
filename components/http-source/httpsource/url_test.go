package httpsource

import "testing"

func TestArtifactNameFromRedirectedURL(t *testing.T) {
	name, err := ArtifactName("https://example.test/releases/logger.wasm?download=1")
	if err != nil {
		t.Fatalf("ArtifactName: %v", err)
	}
	if name != "logger" {
		t.Fatalf("name = %q, want %q", name, "logger")
	}
}

func TestArtifactNameRejectsEmptyFilename(t *testing.T) {
	if _, err := ArtifactName("https://example.test/releases/"); err == nil {
		t.Fatal("expected error for URL without a filename")
	}
}

func TestNormalizeURLRejectsCrossSchemeResolution(t *testing.T) {
	if _, err := NormalizeURL("https", "http://example.test/plugin.wasm"); err == nil {
		t.Fatal("expected error for cross-scheme resolution")
	}
	url, err := NormalizeURL("https", "//example.test/plugin.wasm")
	if err != nil {
		t.Fatalf("NormalizeURL: %v", err)
	}
	if url != "https://example.test/plugin.wasm" {
		t.Fatalf("url = %q", url)
	}
	if _, err := NormalizeURL("https", "example.test/plugin.wasm"); err == nil {
		t.Fatal("expected error for spec without an authority separator")
	}
}
