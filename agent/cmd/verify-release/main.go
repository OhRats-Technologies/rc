package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/OhRats-Technologies/rc/agent/releaseverify"
)

func main() {
	if len(os.Args) != 5 {
		fatal("usage: verify-release MANIFEST SIGNATURE BIN_DIR VERSION")
	}
	manifestBytes, err := os.ReadFile(os.Args[1])
	if err != nil { fatal("read manifest: %v", err) }
	signatureBytes, err := os.ReadFile(os.Args[2])
	if err != nil { fatal("read signature: %v", err) }
	manifest, err := releaseverify.Verify(manifestBytes, signatureBytes)
	if err != nil { fatal("verify manifest: %v", err) }
	if manifest.Version != os.Args[4] { fatal("manifest version %s does not match %s", manifest.Version, os.Args[4]) }
	for _, artifact := range manifest.Artifacts {
		hash, err := releaseverify.FileSHA256(filepath.Join(os.Args[3], artifact.Name))
		if err != nil { fatal("hash %s: %v", artifact.Name, err) }
		if hash != artifact.SHA256 { fatal("hash mismatch for %s", artifact.Name) }
	}
	fmt.Printf("verified signed RC release %s\n", manifest.Version)
}

func fatal(format string, values ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(1)
}
