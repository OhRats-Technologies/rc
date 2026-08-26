package releaseinfo

import "testing"

const validManifest = `{
  "version":"0.12.0",
  "artifacts":[
    {"os":"darwin","arch":"amd64","name":"rc-darwin-amd64","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
    {"os":"darwin","arch":"arm64","name":"rc-darwin-arm64","sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
    {"os":"linux","arch":"amd64","name":"rc-linux-amd64","sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
    {"os":"linux","arch":"arm64","name":"rc-linux-arm64","sha256":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}
  ]
}`

func TestParseManifest(t *testing.T) {
	manifest, err := Parse([]byte(validManifest))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "0.12.0" || len(manifest.Artifacts) != 4 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestParseRejectsInvalidArtifact(t *testing.T) {
	bad := []byte(`{"version":"0.12.0","artifacts":[]}`)
	if _, err := Parse(bad); err == nil {
		t.Fatal("invalid manifest was accepted")
	}
}

func TestVersionComparisonRejectsRollback(t *testing.T) {
	comparison, err := CompareVersions("0.11.0", "0.12.0")
	if err != nil {
		t.Fatal(err)
	}
	if comparison >= 0 {
		t.Fatal("older release did not compare lower")
	}
}
