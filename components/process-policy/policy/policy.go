// Package policy holds the process admission rules shared by the exported
// process-policy functions. The rules mirror the process-policy component
// contract: execute scope is required, identifiers are bounded tokens, and
// channel/lifetime/terminal combinations must stay coherent.
package policy

import (
	"errors"
	"strings"

	"wit_component/ohrats_rc_process_types"
)

const maxTerminalName = 128

// AuthorizeAccess requires execute scope and either owner authority or
// ownership of the target process.
func AuthorizeAccess(request ohrats_rc_process_types.AccessRequest) error {
	if !request.Principal.CanExecute {
		return errors.New("execute scope required")
	}
	if err := ValidateToken(request.ExecutionId, 128, "execution id"); err != nil {
		return err
	}
	if request.Principal.Role != "owner" && request.OwnerUserId != request.Principal.UserId {
		return errors.New("process access denied")
	}
	return nil
}

// ValidateToken accepts non-empty bounded tokens made of ASCII alphanumerics
// and '-', '_', '.' only.
func ValidateToken(value string, maximum int, label string) error {
	valid := value != "" && len(value) <= maximum
	if valid {
		for index := 0; index < len(value); index++ {
			byte := value[index]
			if !isTokenByte(byte) {
				valid = false
				break
			}
		}
	}
	if valid {
		return nil
	}
	return errors.New("invalid " + label)
}

func isTokenByte(value byte) bool {
	switch {
	case value >= '0' && value <= '9':
		return true
	case value >= 'a' && value <= 'z':
		return true
	case value >= 'A' && value <= 'Z':
		return true
	case value == '-' || value == '_' || value == '.':
		return true
	default:
		return false
	}
}

// ValidateChannel rejects channel/lifetime confusion and terminal requests on
// channels that cannot carry a terminal.
func ValidateChannel(request ohrats_rc_process_types.StartRequest) error {
	valid := false
	switch request.Channel {
	case ohrats_rc_process_types.Channel_Control:
		valid = request.Lifetime == ohrats_rc_process_types.Lifetime_Attached ||
			request.Lifetime == ohrats_rc_process_types.Lifetime_Managed
	case ohrats_rc_process_types.Channel_Ssh, ohrats_rc_process_types.Channel_Mcp:
		valid = request.Lifetime == ohrats_rc_process_types.Lifetime_Managed
	case ohrats_rc_process_types.Channel_Schedule:
		valid = request.Lifetime == ohrats_rc_process_types.Lifetime_Scheduled
	}
	if !valid {
		return errors.New("execution channel and lifetime are incompatible")
	}
	if request.Terminal.IsSome() &&
		(request.Channel == ohrats_rc_process_types.Channel_Mcp ||
			request.Channel == ohrats_rc_process_types.Channel_Schedule) {
		return errors.New("execution channel does not support a terminal")
	}
	return nil
}

// ValidateMode keeps exact argv, portable RC Shell source, and native
// system-shell commands distinct while rejecting NUL bytes and empty input.
func ValidateMode(mode ohrats_rc_process_types.ExecutionMode) error {
	valid := false
	switch mode.Tag() {
	case ohrats_rc_process_types.ExecutionMode_Argv:
		argv := mode.Argv()
		valid = argv.F0 != "" && !strings.ContainsRune(argv.F0, 0)
		if valid {
			for _, arg := range argv.F1 {
				if strings.ContainsRune(arg, 0) {
					valid = false
					break
				}
			}
		}
	case ohrats_rc_process_types.ExecutionMode_RcShell:
		valid = validShellSource(mode.RcShell())
	case ohrats_rc_process_types.ExecutionMode_SystemShell:
		valid = validShellSource(mode.SystemShell())
	case ohrats_rc_process_types.ExecutionMode_SystemLoginShell:
		valid = true
	}
	if valid {
		return nil
	}
	return errors.New("invalid execution mode")
}

func validShellSource(source string) bool {
	return strings.TrimSpace(source) != "" && !strings.ContainsRune(source, 0)
}

// ValidateEnvironment rejects empty, malformed, NUL-bearing, or conflicting
// environment changes while keeping name comparison case-sensitive.
func ValidateEnvironment(environment ohrats_rc_process_types.Environment) error {
	names := make(map[string]struct{}, len(environment.Changes))
	for _, change := range environment.Changes {
		if change.Name == "" ||
			strings.ContainsAny(change.Name, "=\x00") ||
			(change.Value.IsSome() && strings.ContainsRune(change.Value.Some(), 0)) {
			return errors.New("invalid or conflicting environment change")
		}
		if _, seen := names[change.Name]; seen {
			return errors.New("invalid or conflicting environment change")
		}
		names[change.Name] = struct{}{}
	}
	if environment.Base != ohrats_rc_process_types.EnvironmentBase_Inherit &&
		environment.Base != ohrats_rc_process_types.EnvironmentBase_Clean {
		return errors.New("invalid or conflicting environment change")
	}
	return nil
}

// NormalizeTerminal validates the terminal geometry and TERM name and
// substitutes the portable default when TERM is blank.
func NormalizeTerminal(value ohrats_rc_process_types.Terminal) (ohrats_rc_process_types.Terminal, error) {
	if !ValidTerminalDimension(value.Cols) ||
		!ValidTerminalDimension(value.Rows) ||
		len(value.Term) > maxTerminalName ||
		strings.ContainsRune(value.Term, 0) {
		return ohrats_rc_process_types.Terminal{}, errors.New("invalid terminal specification")
	}
	term := strings.TrimSpace(value.Term)
	if term == "" {
		term = "xterm-256color"
	}
	return ohrats_rc_process_types.Terminal{Cols: value.Cols, Rows: value.Rows, Term: term}, nil
}

// ValidTerminalDimension bounds terminal geometry to 2..=500 cells.
func ValidTerminalDimension(value uint16) bool {
	return value >= 2 && value <= 500
}
