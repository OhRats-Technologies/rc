package executionruntime

import (
	"errors"
	"strings"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	environmentHost "wit_component/ohrats_rc_process_environment_host"
	processHost "wit_component/ohrats_rc_process_process_host"
	processTypes "wit_component/ohrats_rc_process_types"
)

// spawnRequest resolves an authorized start plan into the exact native spawn
// request. Portable RC Shell never reaches this path.
func spawnRequest(plan processTypes.StartPlan) (processHost.SpawnRequest, error) {
	var program string
	var args []string
	switch plan.Mode.Tag() {
	case processTypes.ExecutionMode_Argv:
		argv := plan.Mode.Argv()
		program = argv.F0
		args = argv.F1
	case processTypes.ExecutionMode_SystemShell:
		resolved, err := systemCommand(plan.Mode.SystemShell(), false)
		if err != nil {
			return processHost.SpawnRequest{}, err
		}
		program, args = resolved.program, resolved.args
	case processTypes.ExecutionMode_SystemLoginShell:
		resolved, err := systemCommand("", true)
		if err != nil {
			return processHost.SpawnRequest{}, err
		}
		program, args = resolved.program, resolved.args
	default:
		return processHost.SpawnRequest{}, errors.New("RC Shell plan reached native spawn")
	}
	return processHost.SpawnRequest{
		Program:     program,
		Args:        args,
		Cwd:         plan.Cwd,
		Environment: plan.Environment,
		Terminal:    plan.Terminal,
	}, nil
}

type invocation struct {
	program string
	args    []string
}

// systemCommand selects the native shell for system-shell and
// system-login-shell modes: valid RC_SHELL, then the enrolled user's shell,
// then /bin/sh on Unix or the discoverable Windows shell family.
func systemCommand(source string, login bool) (invocation, error) {
	environment, err := environmentSnapshot()
	if err != nil {
		return invocation{}, err
	}
	windows := environmentHost.HostPlatform() == environmentHost.PlatformKind_Windows
	shell := ""
	if override := environmentValue(environment, "RC_SHELL", true); override != "" {
		resolved, err := findExecutable(override, environment)
		if err != nil {
			return invocation{}, err
		}
		if resolved == "" {
			return invocation{}, errors.New("RC_SHELL does not resolve to an executable")
		}
		shell = resolved
	} else {
		resolved, err := resolveDefaultShell(environment, windows)
		if err != nil {
			return invocation{}, err
		}
		if resolved == "" {
			return invocation{}, errors.New("no system shell is available")
		}
		shell = resolved
	}
	return shellInvocation(shell, source, login, windows)
}

// shellInvocation maps a resolved shell to its command and login flags. Unknown
// shell families fail on Windows instead of receiving Unix flags.
func shellInvocation(shell string, source string, login bool, windows bool) (invocation, error) {
	name := shell
	if index := strings.LastIndexAny(shell, `/\`); index >= 0 {
		name = shell[index+1:]
	}
	if strings.EqualFold(name, "cmd.exe") || strings.EqualFold(name, "cmd") {
		if login {
			return invocation{program: shell, args: []string{"/D"}}, nil
		}
		return invocation{program: shell, args: []string{"/D", "/S", "/C", source}}, nil
	}
	if strings.EqualFold(name, "pwsh.exe") ||
		strings.EqualFold(name, "pwsh") ||
		strings.EqualFold(name, "powershell.exe") {
		args := []string{"-NoLogo"}
		if !login {
			args = append(args, "-NonInteractive", "-Command", source)
		}
		return invocation{program: shell, args: args}, nil
	}
	if windows {
		return invocation{}, errors.New("RC_SHELL on Windows must select cmd.exe, pwsh.exe, or powershell.exe")
	}
	if login {
		return invocation{program: shell, args: []string{"-l"}}, nil
	}
	return invocation{program: shell, args: []string{"-lc", source}}, nil
}

func resolveDefaultShell(environment []witTypes.Tuple2[string, string], windows bool) (string, error) {
	if windows {
		for _, candidate := range []string{"pwsh.exe", "powershell.exe", "cmd.exe"} {
			resolved, err := findExecutable(candidate, environment)
			if err == nil && resolved != "" {
				return resolved, nil
			}
		}
		return "", nil
	}
	if configured := environmentValue(environment, "SHELL", false); configured != "" {
		resolved, err := findExecutable(configured, environment)
		if err != nil {
			return "", err
		}
		if resolved != "" {
			return resolved, nil
		}
	}
	return findExecutable("/bin/sh", environment)
}

func environmentSnapshot() ([]witTypes.Tuple2[string, string], error) {
	snapshot := environmentHost.Snapshot()
	if snapshot.IsErr() {
		return nil, errors.New(snapshot.Err())
	}
	return snapshot.Ok(), nil
}

func findExecutable(program string, environment []witTypes.Tuple2[string, string]) (string, error) {
	found := environmentHost.FindExecutable(program, environment)
	if found.IsErr() {
		return "", errors.New(found.Err())
	}
	return found.Ok().SomeOr(""), nil
}

// environmentValue returns the first non-blank value for name. Windows compares
// names case-insensitively; Unix compares them exactly.
func environmentValue(environment []witTypes.Tuple2[string, string], name string, foldCase bool) string {
	for _, entry := range environment {
		matches := entry.F0 == name
		if foldCase {
			matches = strings.EqualFold(entry.F0, name)
		}
		if matches && strings.TrimSpace(entry.F1) != "" {
			return entry.F1
		}
	}
	return ""
}
