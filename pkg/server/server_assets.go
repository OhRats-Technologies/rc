package server

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/version"
)

func computeAssetVersion(assetsDir string) string {
	data, err := os.ReadFile(filepath.Join(assetsDir, "styles.css"))
	if err != nil || len(data) == 0 {
		return version.Version
	}
	sum := sha256.Sum256(data)
	return version.Version + "-" + hex.EncodeToString(sum[:6])
}

func (s *Server) assetsVersion() string {
	if s.assetVer != "" {
		return s.assetVer
	}
	return version.Version
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) handleAssets(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/assets/")
	if rel == "" || strings.HasPrefix(rel, "/") || !filepath.IsLocal(filepath.FromSlash(rel)) {
		http.NotFound(w, r)
		return
	}
	fullPath := filepath.Join(s.cfg.AssetsDir, filepath.FromSlash(path.Clean(rel)))
	fi, err := os.Stat(fullPath)
	if err != nil || fi.IsDir() {
		http.NotFound(w, r)
		return
	}
	if contentHashed(path.Base(fullPath)) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	}
	http.ServeFile(w, r, fullPath)
}

// contentHashed reports names produced by the client bundler's content-hash
// scheme. Other assets keep a stable name and must be revalidated.
func contentHashed(name string) bool {
	if strings.HasPrefix(name, "chunk-") {
		return true
	}
	extensions := map[string]bool{
		"css": true, "js": true, "mjs": true, "png": true, "jpg": true, "jpeg": true,
		"gif": true, "svg": true, "webp": true, "ico": true, "woff": true, "woff2": true,
		"ttf": true, "otf": true, "json": true, "map": true, "txt": true, "wasm": true,
	}
	for index := 0; index < len(name); index++ {
		if name[index] != '.' {
			continue
		}
		digest := name[index+1:]
		if end := strings.IndexByte(digest, '.'); end >= 0 {
			digest = digest[:end]
		}
		if len(digest) >= 6 && !extensions[digest] {
			return true
		}
	}
	return false
}
