//go:build !windows

package update

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/OhRats-Technologies/rc/pkg/platform"
)

const replacementJournal = ".rc-replacement.journal"

func stageBinary(binDir string, data []byte) (string, error) {
	if err := ensurePrivateDir(binDir); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(binDir, ".rc-stage-")
	if err != nil {
		return "", err
	}
	name := file.Name()
	if err := file.Chmod(0o755); err != nil {
		file.Close()
		os.Remove(name)
		return "", err
	}
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

func commitBinary(binDir, staged, digest, _ string) error {
	if err := ensurePrivateDir(binDir); err != nil {
		return err
	}
	if err := recoverPending(binDir); err != nil {
		return err
	}
	target := filepath.Join(binDir, platform.ExecutableName("rc"))
	backup := filepath.Join(binDir, ".rc-backup-"+strconv.Itoa(os.Getpid()))
	if _, err := os.Stat(target); err == nil {
		if err := copyExecutable(target, backup); err != nil {
			return err
		}
	}
	journal := filepath.Join(binDir, replacementJournal)
	content := strings.Join([]string{staged, backup, digest}, "\n") + "\n"
	if err := writeFileAtomic(journal, []byte(content), 0o600); err != nil {
		return err
	}
	if err := os.Rename(staged, target); err != nil {
		_ = os.Remove(journal)
		_ = os.Remove(backup)
		return fmt.Errorf("atomically replace RC: %w", err)
	}
	if err := syncDir(binDir); err != nil {
		return err
	}
	_ = os.Remove(journal)
	_ = os.Remove(backup)
	return nil
}

func recoverPending(binDir string) error {
	journal := filepath.Join(binDir, replacementJournal)
	data, err := os.ReadFile(journal)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		return errors.New("invalid RC replacement journal")
	}
	stage, backup, expected := lines[0], lines[1], lines[2]
	target := filepath.Join(binDir, platform.ExecutableName("rc"))
	if current, err := os.ReadFile(target); err == nil {
		if err := VerifyDigest(current, expected); err == nil {
			_ = os.Remove(stage)
			_ = os.Remove(backup)
			_ = os.Remove(journal)
			return syncDir(binDir)
		}
	}
	if _, err := os.Stat(backup); err == nil {
		if err := os.Rename(backup, target); err != nil {
			return fmt.Errorf("restore RC backup: %w", err)
		}
	}
	_ = os.Remove(stage)
	_ = os.Remove(journal)
	if err := syncDir(binDir); err != nil {
		return err
	}
	if _, err := os.Stat(target); err != nil {
		return errors.New("RC replacement was interrupted and no backup is available")
	}
	return nil
}

func replaceProcess(executable string) error {
	args := append([]string{executable}, os.Args[1:]...)
	return syscall.Exec(executable, args, os.Environ())
}
