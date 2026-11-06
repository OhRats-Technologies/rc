package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProtectAndValidatePrivatePath(t *testing.T) {
	temp := t.TempDir()

	dir := filepath.Join(temp, "private_dir")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := ProtectPrivatePath(dir, true); err != nil {
		t.Fatalf("ProtectPrivatePath on dir failed: %v", err)
	}
	if err := ValidatePrivatePath(dir, true); err != nil {
		t.Fatalf("ValidatePrivatePath on dir failed: %v", err)
	}

	file := filepath.Join(dir, "private_file")
	if err := os.WriteFile(file, []byte("secret"), 0644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	if err := ProtectPrivatePath(file, false); err != nil {
		t.Fatalf("ProtectPrivatePath on file failed: %v", err)
	}
	if err := ValidatePrivatePath(file, false); err != nil {
		t.Fatalf("ValidatePrivatePath on file failed: %v", err)
	}

	// Mismatched directory vs file expectation should fail
	if err := ValidatePrivatePath(file, true); err == nil {
		t.Fatal("expected error validating file as directory")
	}
	if err := ValidatePrivatePath(dir, false); err == nil {
		t.Fatal("expected error validating directory as file")
	}
}
