package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestDigestAndManifest(t *testing.T) {
	data := []byte("rc")
	if got := Digest(data); got != "sha256:7581e0d493f6ab3fc525c4dce2a5f835c0399ebb3207d49e609cbccb2d902a42" {
		t.Fatalf("Digest = %s", got)
	}
	if err := VerifyDigest(data, Digest(data)); err != nil {
		t.Fatal(err)
	}
	if err := VerifyDigest(data, "sha256:0000000000000000000000000000000000000000000000000000000000000000"); err == nil {
		t.Fatal("mismatched digest accepted")
	}
	for _, bad := range []string{"", "sha256:ABC", "md5:abc", "sha256:zz"} {
		if _, err := ParseDigest(bad); err == nil {
			t.Fatalf("ParseDigest(%q) unexpectedly succeeded", bad)
		}
	}

	manifest, err := ParseManifest([]byte(`{"schema":1,"minimumUpgrader":"0.19.5"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := EnforceUpgradeManifest(manifest, "0.20.2"); err != nil {
		t.Fatal(err)
	}
	if err := EnforceUpgradeManifest(manifest, "0.19.4"); err == nil {
		t.Fatal("old upgrader accepted")
	}
	if _, err := ParseManifest([]byte(`{"schema":2,"minimumUpgrader":"0.19.5"}`)); err == nil {
		t.Fatal("unknown schema accepted")
	}
}

func TestExtractSingleRejectsExtraAndUnsafeEntries(t *testing.T) {
	good := tarGz(t, map[string][]byte{"rc": []byte("binary")})
	data, err := ExtractSingle(good, "rc", MaxBinary)
	if err != nil || !bytes.Equal(data, []byte("binary")) {
		t.Fatalf("ExtractSingle = %q, %v", data, err)
	}
	extra := tarGz(t, map[string][]byte{"rc": []byte("binary"), "other": []byte("x")})
	if _, err := ExtractSingle(extra, "rc", MaxBinary); err == nil {
		t.Fatal("archive with extra member accepted")
	}
	unsafe := tarGz(t, map[string][]byte{"../rc": []byte("binary")})
	if _, err := ExtractSingle(unsafe, "rc", MaxBinary); err == nil {
		t.Fatal("unsafe path accepted")
	}
}

func TestExtractProfileVerifiesLock(t *testing.T) {
	component := []byte("\x00asm\x01\x00\x00\x00")
	lock := "schema 1\nprofile ohrats:core\ncomponent crypto-control " + Digest(component) + "\n"
	archive := tarGz(t, map[string][]byte{
		"profile.lock":                   []byte(lock),
		"components/crypto-control.wasm": component,
	})
	profile, err := ExtractProfile(archive, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(profile.Components) != 1 || profile.Digests["crypto-control"] != Digest(component) {
		t.Fatalf("unexpected profile: %+v", profile)
	}

	tampered := "schema 1\nprofile ohrats:core\ncomponent crypto-control " + Digest([]byte("other")) + "\n"
	if _, err := ExtractProfile(tarGz(t, map[string][]byte{
		"profile.lock":                   []byte(tampered),
		"components/crypto-control.wasm": component,
	}), true); err == nil {
		t.Fatal("tampered profile accepted")
	}
	if _, err := ExtractProfile(tarGz(t, map[string][]byte{
		"components/crypto-control.wasm": component,
	}), true); err == nil {
		t.Fatal("profile without lock accepted")
	}
}

func TestExtractProfileAcceptsDirectoryEntry(t *testing.T) {
	component := []byte("\x00asm\x01\x00\x00\x00")
	lock := "schema 1\nprofile ohrats:core\ncomponent crypto-control " + Digest(component) + "\n"
	var buffer bytes.Buffer
	gw := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{Name: "components/", Mode: 0o755, Typeflag: tar.TypeDir})
	_ = tw.WriteHeader(&tar.Header{Name: "profile.lock", Mode: 0o600, Size: int64(len(lock)), Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte(lock))
	_ = tw.WriteHeader(&tar.Header{Name: "components/crypto-control.wasm", Mode: 0o644, Size: int64(len(component)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(component)
	_ = tw.Close()
	_ = gw.Close()

	profile, err := ExtractProfile(buffer.Bytes(), true)
	if err != nil {
		t.Fatalf("expected profile with components/ directory entry to succeed, got %v", err)
	}
	if len(profile.Components) != 1 {
		t.Fatalf("unexpected components count: %d", len(profile.Components))
	}
}
