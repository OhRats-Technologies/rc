package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OhRats-Technologies/rc/agent/releaseverify"
)

func main() {
	if len(os.Args) != 6 {
		fatal("usage: sign-release PRIVATE_KEY BIN_DIR VERSION MANIFEST SIGNATURE")
	}
	privateKey := loadKey(os.Args[1])
	binDir, version, manifestPath, signaturePath := os.Args[2], os.Args[3], os.Args[4], os.Args[5]
	manifest := releaseverify.Manifest{Version: version}
	for _, target := range [][2]string{{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}} {
		name := "ohrats-rc-" + target[0] + "-" + target[1]
		hash, err := releaseverify.FileSHA256(filepath.Join(binDir, name))
		if err != nil {
			fatal("hash %s: %v", name, err)
		}
		manifest.Artifacts = append(manifest.Artifacts, releaseverify.Artifact{OS: target[0], Arch: target[1], Name: name, SHA256: hash})
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		fatal("manifest: %v", err)
	}
	data = append(data, '\n')
	signature := ed25519.Sign(privateKey, data)
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		fatal("write manifest: %v", err)
	}
	encoded := append([]byte(base64.RawURLEncoding.EncodeToString(signature)), '\n')
	if err := os.WriteFile(signaturePath, encoded, 0644); err != nil {
		fatal("write signature: %v", err)
	}
}

func loadKey(path string) ed25519.PrivateKey {
	data, err := os.ReadFile(path)
	if err != nil {
		fatal("read private key: %v", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		fatal("invalid private key PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		fatal("parse private key: %v", err)
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		fatal("private key is not Ed25519")
	}
	return key
}

func fatal(format string, values ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(1)
}
