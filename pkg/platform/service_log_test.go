package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTailFollowAndRestartPreserveOrder(t *testing.T) {
	temp := t.TempDir()

	log, err := NewJournal(temp)
	if err != nil {
		t.Fatalf("NewJournal failed: %v", err)
	}
	for n := 0; n < 5; n++ {
		if err := log.Append("line " + string(rune('0'+n))); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	tail, err := Recent(temp, "", 2)
	if err != nil {
		t.Fatalf("Recent failed: %v", err)
	}
	if len(tail) != 2 || tail[0].Message != "line 3" || tail[1].Message != "line 4" {
		t.Fatalf("unexpected tail: %+v", tail)
	}

	cursor := tail[1].Cursor
	afterTail, err := Recent(temp, cursor, 2)
	if err != nil {
		t.Fatalf("Recent after tail failed: %v", err)
	}
	if len(afterTail) != 0 {
		t.Fatalf("expected empty after tail, got %+v", afterTail)
	}

	// Restart journal
	log2, err := NewJournal(temp)
	if err != nil {
		t.Fatalf("NewJournal restart failed: %v", err)
	}
	for n := 5; n < 10; n++ {
		if err := log2.Append("line " + string(rune('0'+n))); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	next, err := Recent(temp, cursor, 2)
	if err != nil {
		t.Fatalf("Recent failed: %v", err)
	}
	if len(next) != 2 || next[0].Message != "line 5" || next[1].Message != "line 6" {
		t.Fatalf("unexpected next entries: %+v", next)
	}

	afterNext, err := Recent(temp, next[1].Cursor, 2)
	if err != nil {
		t.Fatalf("Recent after next failed: %v", err)
	}
	if len(afterNext) == 0 || afterNext[0].Message != "line 7" {
		t.Fatalf("unexpected after next: %+v", afterNext)
	}
}

func TestPartialRecordsAreIgnoredAndMessagesCannotInjectRecords(t *testing.T) {
	temp := t.TempDir()

	log, err := NewJournal(temp)
	if err != nil {
		t.Fatalf("NewJournal failed: %v", err)
	}
	if err := log.Append("hello\n\t\x1bworld 🐀"); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	entries, err := Recent(temp, "", 20)
	if err != nil {
		t.Fatalf("Recent failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Message != "hello   world 🐀" {
		t.Fatalf("unexpected sanitized message: %+v", entries)
	}

	// Append corrupt partial record to file
	current := filepath.Join(temp, "service.log")
	f, err := os.OpenFile(current, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("open file failed: %v", err)
	}
	_, _ = f.Write([]byte("partial\t123\t\xf0\x9f"))
	f.Close()

	entries2, err := Recent(temp, "", 20)
	if err != nil {
		t.Fatalf("Recent failed: %v", err)
	}
	if len(entries2) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries2))
	}

	if _, err := Recent(temp, "", 0); err == nil {
		t.Fatal("expected error for limit 0")
	}

	// NewJournal recovers truncated record
	log3, err := NewJournal(temp)
	if err != nil {
		t.Fatalf("NewJournal recovery failed: %v", err)
	}
	if err := log3.Append("recovered"); err != nil {
		t.Fatalf("Append after recovery failed: %v", err)
	}

	entries3, err := Recent(temp, "", 20)
	if err != nil {
		t.Fatalf("Recent failed: %v", err)
	}
	if len(entries3) != 2 || entries3[1].Message != "recovered" {
		t.Fatalf("unexpected entries after recovery: %+v", entries3)
	}
}

func TestRotationIsBoundedAndFollowCrossesFiles(t *testing.T) {
	temp := t.TempDir()

	log, err := NewJournal(temp)
	if err != nil {
		t.Fatalf("NewJournal failed: %v", err)
	}

	text := strings.Repeat("x", MaxMessage)
	current := filepath.Join(temp, "service.log")
	for {
		fi, err := os.Stat(current)
		if err == nil && fi.Size() >= logCapacity {
			break
		}
		if err := log.Append(text); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	entries, err := Recent(temp, "", 1)
	if err != nil || len(entries) == 0 {
		t.Fatalf("Recent failed: %v", err)
	}
	cursor := entries[0].Cursor

	if err := log.Append("after rotation"); err != nil {
		t.Fatalf("Append after rotation failed: %v", err)
	}

	recentAfter, err := Recent(temp, cursor, 10)
	if err != nil || len(recentAfter) == 0 {
		t.Fatalf("Recent after rotation failed: %v", err)
	}
	if recentAfter[0].Message != "after rotation" {
		t.Fatalf("expected 'after rotation', got %s", recentAfter[0].Message)
	}
}
