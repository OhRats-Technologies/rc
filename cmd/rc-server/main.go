package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/server"
	"github.com/OhRats-Technologies/rc/pkg/version"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Printf("RC server %s\n", version.Version)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		healthcheck()
		return
	}
	addr := os.Getenv("RC_ADDR")
	if addr == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		addr = ":" + port
	}

	dataDir := firstNonEmpty(os.Getenv("RC_DATA_DIR"), os.Getenv("DATA_DIR"))
	if dataDir == "" {
		dataDir = "./data"
	}

	assetsDir := firstNonEmpty(os.Getenv("RC_ASSETS_DIR"), os.Getenv("STATIC_DIR"))
	if assetsDir == "" {
		assetsDir = "dist/assets"
	}

	cfg := server.Config{
		Addr:                   addr,
		DataDir:                dataDir,
		AssetsDir:              assetsDir,
		DBPath:                 os.Getenv("RC_DB_PATH"),
		SetupToken:             os.Getenv("RC_SETUP_TOKEN"),
		PublicURL:              firstNonEmpty(os.Getenv("PUBLIC_URL"), os.Getenv("RC_PUBLIC_URL")),
		PublicSignup:           os.Getenv("RC_PUBLIC_SIGNUP") == "1",
		Development:            os.Getenv("RC_DEV_INSECURE") == "1",
		DevelopmentEnrollToken: os.Getenv("RC_DEV_ENROLL_TOKEN"),
		TurnstileSiteKey:       strings.TrimSpace(os.Getenv("RC_TURNSTILE_SITE_KEY")),
		TurnstileSecretKey:     strings.TrimSpace(os.Getenv("RC_TURNSTILE_SECRET_KEY")),
		TrustProxy:             os.Getenv("RC_TRUST_PROXY") == "1",
		TurnTokenID:            strings.TrimSpace(os.Getenv("RC_CF_TURN_TOKEN_ID")),
		TurnAPIToken:           strings.TrimSpace(os.Getenv("RC_CF_TURN_API_TOKEN")),
		MCPAccessTTLMinutes:    positiveUint("RC_MCP_ACCESS_TTL_MINUTES", 15),
	}
	if cfg.Development {
		log.Print("RC_DEV_INSECURE=1: exposing the in-memory development control plane")
	}

	srv := server.New(cfg)

	ln, err := srv.Listen()
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	fmt.Printf("RC Server listening on %s\n", ln.Addr())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := srv.Serve(ln); err != nil {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-stop
	fmt.Println("\nShutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func healthcheck() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		log.Fatalf("healthcheck: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Fatalf("healthcheck: status %d", res.StatusCode)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func positiveUint(key string, fallback uint64) uint64 {
	value, err := strconv.ParseUint(strings.TrimSpace(os.Getenv(key)), 10, 64)
	if err != nil || value == 0 {
		return fallback
	}
	return value
}
