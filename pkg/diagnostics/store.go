// Package diagnostics stores bounded local component/runtime events.
package diagnostics

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxBytes   = 1 << 20
	maxEntries = 512
)

// Event is one bounded local diagnostic record. It never carries command
// text, cwd, environment values, streams, transcripts, credentials, or keys.
type Event struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Source    string `json:"source"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

// Path returns the diagnostics journal path for a state directory.
func Path(stateDir string) string {
	return filepath.Join(stateDir, "diagnostics.jsonl")
}

// Append writes one event, rotating the journal when it grows past its bound.
func Append(stateDir string, event Event) error {
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	event.Message = sanitize(event.Message)
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	path := Path(stateDir)
	if info, err := os.Stat(path); err == nil && info.Size()+int64(len(line)) > maxBytes {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(line)
	return err
}

// Recent returns at most limit events, newest last.
func Recent(stateDir string, limit int) ([]Event, error) {
	if limit < 1 {
		return nil, nil
	}
	if limit > maxEntries {
		limit = maxEntries
	}
	var events []Event
	for _, name := range []string{"diagnostics.jsonl.1", "diagnostics.jsonl"} {
		file, err := os.Open(filepath.Join(stateDir, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 4096), 64<<10)
		for scanner.Scan() {
			var event Event
			if json.Unmarshal(scanner.Bytes(), &event) != nil {
				continue
			}
			events = append(events, event)
			if len(events) > limit*4 {
				events = events[len(events)-limit*4:]
			}
		}
		file.Close()
		if err := scanner.Err(); err != nil {
			return nil, err
		}
	}
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events, nil
}

func sanitize(message string) string {
	message = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, message)
	if len(message) > 512 {
		message = message[:512]
	}
	return message
}
