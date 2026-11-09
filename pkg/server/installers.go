package server

import (
	"net/http"
	"os"
	"path/filepath"
)

// handleInstaller serves the public release installers from the asset
// directory so enrollment commands never point at a missing route.
func (s *Server) handleInstaller(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fullPath := filepath.Join(s.cfg.AssetsDir, name)
		fi, err := os.Stat(fullPath)
		if err != nil || fi.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
		http.ServeFile(w, r, fullPath)
	}
}
