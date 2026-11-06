package executionruntime

import (
	"reflect"
	"testing"
)

func TestShellInvocationMapsNativeShellFamilies(t *testing.T) {
	cmd, err := shellInvocation("cmd.exe", "echo hi", false, true)
	if err != nil {
		t.Fatalf("cmd.exe: %v", err)
	}
	if !reflect.DeepEqual(cmd.args, []string{"/D", "/S", "/C", "echo hi"}) {
		t.Fatalf("cmd.exe args = %v", cmd.args)
	}
	pwsh, err := shellInvocation("pwsh.exe", "", true, true)
	if err != nil {
		t.Fatalf("pwsh.exe: %v", err)
	}
	if !reflect.DeepEqual(pwsh.args, []string{"-NoLogo"}) {
		t.Fatalf("pwsh.exe login args = %v", pwsh.args)
	}
	zsh, err := shellInvocation("/bin/zsh", "", true, false)
	if err != nil {
		t.Fatalf("/bin/zsh: %v", err)
	}
	if !reflect.DeepEqual(zsh.args, []string{"-l"}) {
		t.Fatalf("/bin/zsh login args = %v", zsh.args)
	}
	if _, err := shellInvocation("custom.exe", "", true, true); err == nil {
		t.Fatal("unknown Windows shell family was accepted")
	}
}

func TestReplayPositionsAreBoundedAndDomainSeparated(t *testing.T) {
	values := replayPositions("execution-1")
	if len(values) != replayHashes {
		t.Fatalf("positions = %d, want %d", len(values), replayHashes)
	}
	for _, value := range values {
		if value.chunk < 0 || value.chunk >= replayChunks {
			t.Fatalf("chunk %d out of range", value.chunk)
		}
		if value.bit < 0 || value.bit >= replayChunkBytes*8 {
			t.Fatalf("bit %d out of range", value.bit)
		}
	}
	if reflect.DeepEqual(values, replayPositions("execution-2")) {
		t.Fatal("distinct ids produced identical replay positions")
	}
}
