package platform

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
)

const (
	logCapacity = 1024 * 1024
	MaxMessage  = 4096
)

var journalGeneration atomic.Uint64

// LogEntry is an entry read from the service journal.
type LogEntry struct {
	Cursor    string `json:"cursor"`
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
}

// Journal manages bounded appending to the service log.
type Journal struct {
	directory  string
	sequence   uint64
	generation string
}

// NewJournal initializes the journal in the given directory.
func NewJournal(directory string) (*Journal, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	current := filepath.Join(directory, "service.log")
	if data, err := os.ReadFile(current); err == nil && len(data) > 0 {
		if data[len(data)-1] != '\n' {
			idx := bytes.LastIndexByte(data, '\n')
			newLen := int64(0)
			if idx >= 0 {
				newLen = int64(idx + 1)
			}
			_ = os.Truncate(current, newLen)
		}
	}
	return &Journal{
		directory:  directory,
		sequence:   0,
		generation: fmt.Sprintf("%d-%d-%d", Micros(), os.Getpid(), journalGeneration.Add(1)),
	}, nil
}

// Append sanitizes message and appends a record to service.log, rotating if capacity is reached.
func (j *Journal) Append(message string) error {
	current := filepath.Join(j.directory, "service.log")
	if fi, err := os.Stat(current); err == nil && fi.Size() >= logCapacity {
		previous := filepath.Join(j.directory, "service.log.1")
		_ = os.Remove(previous)
		if err := os.Rename(current, previous); err != nil {
			return err
		}
	}

	j.sequence++
	var sb strings.Builder
	count := 0
	for _, r := range message {
		if count >= MaxMessage {
			break
		}
		if unicode.IsControl(r) {
			sb.WriteRune(' ')
		} else {
			sb.WriteRune(r)
		}
		count++
	}

	line := fmt.Sprintf("%s-%d\t%d\t%s\n", j.generation, j.sequence, Micros(), sb.String())
	f, err := os.OpenFile(current, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(line)
	return err
}

// Recent reads up to limit log entries starting after cursor.
func Recent(directory string, cursor string, limit int) ([]LogEntry, error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("invalid log limit")
	}

	for attempt := 0; attempt < 2; attempt++ {
		var entries []LogEntry
		seen := make(map[string]bool)

		files := []string{"service.log.1", "service.log"}
		for _, name := range files {
			f, err := os.Open(filepath.Join(directory, name))
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return nil, err
			}
			lr := io.LimitReader(f, int64(logCapacity+(MaxMessage*4+256)))
			data, readErr := io.ReadAll(lr)
			f.Close()
			if readErr != nil {
				return nil, readErr
			}

			lines := bytes.Split(data, []byte("\n"))
			// Discard last element if file did not end with \n
			if len(data) > 0 && data[len(data)-1] != '\n' {
				lines = lines[:len(lines)-1]
			}

			for _, l := range lines {
				if len(l) == 0 {
					continue
				}
				parts := strings.SplitN(string(l), "\t", 3)
				if len(parts) == 3 {
					c, ts, msg := parts[0], parts[1], parts[2]
					if seen[c] {
						continue
					}
					seen[c] = true
					entries = append(entries, LogEntry{
						Cursor:    c,
						Timestamp: ts,
						Message:   msg,
					})
				}
			}
		}

		start := 0
		if cursor == "" {
			if len(entries) > limit {
				start = len(entries) - limit
			}
		} else {
			pos := -1
			for i, e := range entries {
				if e.Cursor == cursor {
					pos = i
					break
				}
			}
			if pos != -1 {
				start = pos + 1
			} else if attempt == 0 {
				continue
			} else {
				start = 0 // Cursor rotated out
			}
		}

		end := start + limit
		if end > len(entries) {
			end = len(entries)
		}
		if start > len(entries) {
			start = len(entries)
		}

		result := make([]LogEntry, end-start)
		copy(result, entries[start:end])
		return result, nil
	}
	return nil, nil
}

// Micros returns current epoch microseconds.
func Micros() int64 {
	return time.Now().UnixMicro()
}
