package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func mergeEnv(base, overrides []string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	order := make([]string, 0, len(base)+len(overrides))
	for _, pair := range append(append([]string{}, base...), overrides...) {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		if _, seen := values[key]; !seen {
			order = append(order, key)
		}
		values[key] = value
	}
	merged := make([]string, 0, len(order))
	for _, key := range order {
		merged = append(merged, key+"="+values[key])
	}
	return merged
}

// repoRoot walks up from the package directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repository root")
		}
		dir = parent
	}
}

func runCommand(t *testing.T, dir string, env []string, name string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = mergeEnv(os.Environ(), env)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

var (
	buildMu    sync.Mutex
	buildCache = map[string]string{}
)

// buildBinary compiles one cmd package once per test process.
func buildBinary(t *testing.T, pkg, name string) string {
	t.Helper()
	buildMu.Lock()
	defer buildMu.Unlock()
	if path, ok := buildCache[pkg]; ok {
		return path
	}
	dir, err := os.MkdirTemp("", "rc-itest-bin-")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	out := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	stdout, stderr, err := runCommand(t, repoRoot(t), nil, "go", "build", "-o", out, pkg)
	if err != nil {
		t.Fatalf("build %s: %v\n%s%s", pkg, err, stdout, stderr)
	}
	buildCache[pkg] = out
	return out
}

// componentArtifacts ensures dist/components/<name>.wasm exists and copies the
// requested components into an isolated directory.
func componentArtifacts(t *testing.T, names ...string) string {
	t.Helper()
	root := repoRoot(t)
	if _, err := exec.LookPath("componentize-go"); err != nil {
		t.Skip("componentize-go is not installed; skipping component artifact test")
	}
	dest := t.TempDir()
	for _, name := range names {
		source := filepath.Join(root, "dist", "components", name+".wasm")
		if _, err := os.Stat(source); err != nil {
			stdout, stderr, buildErr := runCommand(t, root, nil, "sh", "scripts/build-component.sh", "components/"+name)
			if buildErr != nil {
				t.Fatalf("build component %s: %v\n%s%s", name, buildErr, stdout, stderr)
			}
			built := strings.TrimSpace(stdout)
			if !filepath.IsAbs(built) {
				built = filepath.Join(root, built)
			}
			source = built
		}
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(dest, name+".wasm"), data, 0o644); err != nil {
			t.Fatalf("copy %s: %v", name, err)
		}
	}
	return dest
}

type serverProcess struct {
	baseURL string
	cmd     *exec.Cmd
	logs    *bytes.Buffer
	cancel  context.CancelFunc
}

func (s *serverProcess) stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
			_ = s.cmd.Process.Kill()
		}
		done := make(chan struct{})
		go func() { _ = s.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			_ = s.cmd.Process.Kill()
		}
	}
}

// startServer launches cmd/rc-server on a kernel-assigned port and waits for
// /healthz. It returns the resolved base URL, so no fixed ports are used.
// Options are KEY=VALUE pairs; later values replace the defaults.
func startServer(t *testing.T, options ...string) *serverProcess {
	t.Helper()
	binary := buildBinary(t, "./cmd/rc-server", "rc-server")
	values := map[string]string{
		"RC_ADDR":       "127.0.0.1:0",
		"RC_DATA_DIR":   t.TempDir(),
		"RC_ASSETS_DIR": t.TempDir(),
	}
	for _, option := range options {
		key, value, ok := strings.Cut(option, "=")
		if !ok {
			t.Fatalf("invalid server option %q", option)
		}
		values[key] = value
	}
	cmd := exec.Command(binary)
	cmd.Dir = repoRoot(t)
	cmd.Env = os.Environ()
	for key, value := range values {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	logs := &bytes.Buffer{}
	cmd.Stderr = logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("start rc-server: %v", err)
	}

	addrCh := make(chan string, 1)
	go func() {
		buf := make([]byte, 0, 256)
		one := make([]byte, 1)
		for {
			n, readErr := stdout.Read(one)
			if n > 0 {
				buf = append(buf, one[0])
				if strings.Contains(string(buf), "\n") {
					line := strings.TrimSpace(string(buf))
					if idx := strings.Index(line, "listening on "); idx >= 0 {
						select {
						case addrCh <- strings.TrimSpace(line[idx+len("listening on "):]):
						default:
						}
					}
					buf = buf[:0]
				}
			}
			if readErr != nil {
				_, _ = io.Copy(io.Discard, stdout)
				return
			}
		}
	}()

	var addr string
	select {
	case addr = <-addrCh:
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("rc-server did not report a listen address\nstderr:\n%s", logs.String())
	}

	proc := &serverProcess{baseURL: "http://" + addr, cmd: cmd, logs: logs}
	t.Cleanup(proc.stop)
	waitHealthy(t, proc)
	return proc
}

func waitHealthy(t *testing.T, proc *serverProcess) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get(proc.baseURL + "/healthz")
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("rc-server never became healthy\nstderr:\n%s", proc.logs.String())
}

func httpJSON(t *testing.T, method, url string, body any, out any) int {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer res.Body.Close()
	payload, _ := io.ReadAll(res.Body)
	if out != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, out); err != nil {
			t.Fatalf("decode %s %s (%d): %v\n%s", method, url, res.StatusCode, err, payload)
		}
	}
	return res.StatusCode
}

func jsonString(t *testing.T, v any) string {
	t.Helper()
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return fmt.Sprintf("%s", out)
}
