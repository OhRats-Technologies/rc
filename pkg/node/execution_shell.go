package node

import (
	"errors"
	"os"
	"os/exec"
	"runtime"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

// resolveLoginShell picks the interactive shell for a login session. A
// configured shell that does not resolve is ignored: an enrolled Node can
// inherit a SHELL from its parent environment that does not exist in the
// guest, and exec would then fail the whole start.
func resolveLoginShell() string {
	for _, candidate := range []string{os.Getenv("RC_SHELL"), os.Getenv("SHELL")} {
		if candidate == "" {
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			return resolved
		}
	}
	if runtime.GOOS == "windows" {
		for _, candidate := range []string{"pwsh.exe", "powershell.exe", "cmd.exe"} {
			if resolved, err := exec.LookPath(candidate); err == nil {
				return resolved
			}
		}
		return ""
	}
	for _, candidate := range []string{"/bin/sh", "/bin/bash"} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate
		}
	}
	return ""
}

func buildCommand(spec ProcessSpec) (*exec.Cmd, error) {
	var cmd *exec.Cmd

	switch spec.Mode.Kind {
	case protocol.ExecutionModeArgv:
		if spec.Mode.Program == "" {
			return nil, errors.New("missing program for argv execution")
		}
		cmd = exec.Command(spec.Mode.Program, spec.Mode.Args...)

	case protocol.ExecutionModeSystemLoginShell:
		shell := resolveLoginShell()
		if shell == "" {
			return nil, errors.New("no system shell is available")
		}
		if runtime.GOOS == "windows" {
			cmd = exec.Command(shell)
		} else {
			cmd = exec.Command(shell, "-l")
		}

	case protocol.ExecutionModeRcShell, protocol.ExecutionModeSystemShell:
		cmdStr := spec.Mode.Command
		if cmdStr == "" && spec.Mode.Script != "" {
			cmdStr = spec.Mode.Script
		}
		if cmdStr == "" {
			return nil, errors.New("missing command for shell execution")
		}
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd.exe", "/c", cmdStr)
		} else {
			cmd = exec.Command("/bin/sh", "-c", cmdStr)
		}

	default:
		return nil, errors.New("unsupported execution mode")
	}

	if spec.Cwd != "" {
		cmd.Dir = spec.Cwd
	}

	if spec.Environment.Base == protocol.EnvironmentBaseClean {
		cmd.Env = []string{}
	} else {
		cmd.Env = os.Environ()
	}
	for _, change := range spec.Environment.Changes {
		if change.Value != nil {
			cmd.Env = append(cmd.Env, change.Name+"="+*change.Value)
		}
	}

	return cmd, nil
}
