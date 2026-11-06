//go:build windows

package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OhRats-Technologies/rc/pkg/platform"
)

const replacementJournal = ".rc-activation.journal"

func stageBinary(binDir string, data []byte) (string, error) {
	if err := ensurePrivateDir(binDir); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(binDir, ".rc-stage-*.exe")
	if err != nil {
		return "", err
	}
	name := file.Name()
	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(name)
		return "", err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(name)
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

func commitBinary(binDir, staged, digest, releaseVersion string) error {
	if err := ensurePrivateDir(binDir); err != nil {
		return err
	}
	if err := recoverPending(binDir); err != nil {
		return err
	}
	data, err := os.ReadFile(staged)
	if err != nil {
		return err
	}
	versions, err := platform.RuntimeVersionsDir()
	if err != nil {
		return err
	}
	targetDir := filepath.Join(versions, releaseVersion)
	if err := ensurePrivateDir(versions); err != nil {
		return err
	}
	if err := ensurePrivateDir(targetDir); err != nil {
		return err
	}
	kernel := filepath.Join(targetDir, platform.ExecutableName("rc"))
	if err := writeFileAtomic(kernel, data, 0o755); err != nil {
		return err
	}
	_ = os.Remove(staged)

	activeFile, err := platform.RuntimeActivationFile()
	if err != nil {
		return err
	}
	previousFile, err := platform.RuntimePreviousActivationFile()
	if err != nil {
		return err
	}
	previous := ""
	if current, err := os.ReadFile(activeFile); err == nil {
		previous = strings.TrimSpace(string(current))
	}
	journal := filepath.Join(versions, replacementJournal)
	if err := writeFileAtomic(journal, []byte(previous+"\n"+targetDir+"\n"+digest+"\n"), 0o600); err != nil {
		return err
	}
	if err := writeFileAtomic(previousFile, []byte(previous+"\n"), 0o600); err != nil {
		return err
	}
	if err := writeFileAtomic(activeFile, []byte(targetDir+"\n"), 0o600); err != nil {
		return err
	}

	stable := filepath.Join(binDir, platform.ExecutableName("rc"))
	if current, err := os.Executable(); err == nil && samePath(current, stable) {
		if err := spawnPostExitReplace(kernel, stable); err != nil {
			return fmt.Errorf("staged Windows controller replacement: %w; rerun rc upgrade after this process exits", err)
		}
	} else if err := writeFileAtomic(stable, data, 0o755); err != nil {
		return err
	}
	if err := syncDir(versions); err != nil {
		return err
	}
	_ = os.Remove(journal)
	return nil
}

func recoverPending(_ string) error {
	versions, err := platform.RuntimeVersionsDir()
	if err != nil {
		return err
	}
	journal := filepath.Join(versions, replacementJournal)
	data, err := os.ReadFile(journal)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		return errors.New("invalid Windows RC activation journal")
	}
	previous := lines[0]
	activeFile, err := platform.RuntimeActivationFile()
	if err != nil {
		return err
	}
	if previous == "" {
		_ = os.Remove(activeFile)
	} else if err := writeFileAtomic(activeFile, []byte(previous+"\n"), 0o600); err != nil {
		return err
	}
	_ = os.Remove(journal)
	return nil
}

func spawnPostExitReplace(source, destination string) error {
	script := destination + ".replace-" + strconv.Itoa(os.Getpid()) + ".cmd"
	pid := strconv.Itoa(os.Getpid())
	body := strings.Join([]string{
		"@echo off",
		":wait",
		"tasklist /FI \"PID eq " + pid + "\" | find \"" + pid + "\" >nul && (timeout /t 1 /nobreak >nul & goto wait)",
		"copy /Y \"" + source + "\" \"" + destination + "\" >nul",
		"del \"" + script + "\"",
	}, "\r\n")
	if err := os.WriteFile(script, []byte(body), 0o600); err != nil {
		return err
	}
	return exec.Command("cmd", "/c", "start", "/b", script).Start()
}

func samePath(a, b string) bool {
	left, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	right, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return strings.EqualFold(left, right)
}

func replaceProcess(_ string) error {
	return errors.New("re-exec is not supported on Windows; restart the RC service instead")
}
