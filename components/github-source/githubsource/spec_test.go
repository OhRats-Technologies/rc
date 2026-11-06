package githubsource

import "testing"

func TestParseMonorepoSubpathAndReleaseTag(t *testing.T) {
	value, err := Parse("OhRats-Technologies/rc//components/webui#webui-v2.4.1")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if value.Owner != "OhRats-Technologies" || value.Repository != "rc" {
		t.Fatalf("owner/repository = %q/%q", value.Owner, value.Repository)
	}
	if value.Path != "components/webui" {
		t.Fatalf("path = %q", value.Path)
	}
	if value.Revision == nil || *value.Revision != "webui-v2.4.1" {
		t.Fatalf("revision = %v", value.Revision)
	}
	name, err := value.PackageName()
	if err != nil {
		t.Fatalf("PackageName: %v", err)
	}
	if name != "webui" {
		t.Fatalf("name = %q", name)
	}
}

func TestParseRawComponentPathAtRevision(t *testing.T) {
	value, err := Parse("fern/plugins//dist/logger.wasm#abcdef12")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	name, err := value.PackageName()
	if err != nil {
		t.Fatalf("PackageName: %v", err)
	}
	if name != "logger" {
		t.Fatalf("name = %q", name)
	}
	if got := value.Display("abcdef12"); got != "github:fern/plugins//dist/logger.wasm#abcdef12" {
		t.Fatalf("display = %q", got)
	}
}

func TestParseRejectsRepositoryTraversal(t *testing.T) {
	if _, err := Parse("fern/plugins//../private.wasm#main"); err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestSelectAssetPrefersPublishedNames(t *testing.T) {
	assets := []asset{
		{Name: "other.wasm"},
		{Name: "rc-component-logger.wasm"},
	}
	selected, found := selectAsset(assets, "logger")
	if !found || selected.Name != "rc-component-logger.wasm" {
		t.Fatalf("selected = %v, found = %v", selected, found)
	}
	if _, found := selectAsset(assets, "missing"); found {
		t.Fatal("expected no asset for unknown package")
	}
}

func TestDigestComparisonIgnoresHexCase(t *testing.T) {
	expected := "SHA256:ABCD"
	if !equalASCIICaseInsensitive("sha256:abcd", expected) {
		t.Fatal("expected case-insensitive digest comparison")
	}
	if equalASCIICaseInsensitive("sha256:abcd", "sha256:abce") {
		t.Fatal("expected differing digests to mismatch")
	}
}
