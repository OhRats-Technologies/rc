package executionruntime

import (
	"testing"

	processTypes "wit_component/ohrats_rc_process_types"
)

func TestJournalTruncatesPartialChunksAndPreservesStreamBoundaries(t *testing.T) {
	journal := newJournal(6)
	journal.push(processTypes.StreamKind_Stdout, []byte("abc"))
	journal.push(processTypes.StreamKind_Stdout, []byte("de"))
	journal.push(processTypes.StreamKind_Stderr, []byte("XYZ"))
	chunks, cursor, more := journal.read(0, 4)
	if truncated := journal.truncatedBefore(); truncated != 2 {
		t.Fatalf("truncatedBefore = %d, want 2", truncated)
	}
	if string(chunks[0].Bytes) != "cde" {
		t.Fatalf("first chunk = %q, want %q", chunks[0].Bytes, "cde")
	}
	if string(chunks[1].Bytes) != "X" {
		t.Fatalf("second chunk = %q, want %q", chunks[1].Bytes, "X")
	}
	if cursor != 6 {
		t.Fatalf("cursor = %d, want 6", cursor)
	}
	if !more {
		t.Fatal("more = false, want true")
	}
}

func TestJournalOversizedAppendKeepsTailWithAbsoluteCursor(t *testing.T) {
	journal := newJournal(3)
	journal.push(processTypes.StreamKind_Stdout, []byte("abcdef"))
	chunks, cursor, more := journal.read(0, 10)
	if truncated := journal.truncatedBefore(); truncated != 3 {
		t.Fatalf("truncatedBefore = %d, want 3", truncated)
	}
	if chunks[0].Cursor != 3 {
		t.Fatalf("cursor = %d, want 3", chunks[0].Cursor)
	}
	if string(chunks[0].Bytes) != "def" {
		t.Fatalf("chunk = %q, want %q", chunks[0].Bytes, "def")
	}
	if cursor != 6 {
		t.Fatalf("next cursor = %d, want 6", cursor)
	}
	if more {
		t.Fatal("more = true, want false")
	}
}
