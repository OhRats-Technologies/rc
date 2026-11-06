package schedule

import (
	"encoding/json"
	"fmt"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	processTypes "wit_component/ohrats_rc_process_types"
)

// storedMode is the tagged execution-mode union persisted by the Rust model:
// {"kind":"argv","program":...,"args":[...]}, {"kind":"rcShell","script":...},
// {"kind":"systemShell","command":...} or {"kind":"systemLoginShell"}.
type storedMode struct {
	kind    string
	program string
	args    []string
	script  string
	command string
}

func storedModeFromExecutionMode(mode processTypes.ExecutionMode) storedMode {
	switch mode.Tag() {
	case processTypes.ExecutionMode_Argv:
		argv := mode.Argv()
		args := make([]string, len(argv.F1))
		copy(args, argv.F1)
		return storedMode{kind: "argv", program: argv.F0, args: args}
	case processTypes.ExecutionMode_RcShell:
		return storedMode{kind: "rcShell", script: mode.RcShell()}
	case processTypes.ExecutionMode_SystemShell:
		return storedMode{kind: "systemShell", command: mode.SystemShell()}
	default:
		return storedMode{kind: "systemLoginShell"}
	}
}

func executionModeFromStored(mode storedMode) processTypes.ExecutionMode {
	switch mode.kind {
	case "argv":
		return processTypes.MakeExecutionModeArgv(witTypes.Tuple2[string, []string]{
			F0: mode.program,
			F1: mode.args,
		})
	case "rcShell":
		return processTypes.MakeExecutionModeRcShell(mode.script)
	case "systemShell":
		return processTypes.MakeExecutionModeSystemShell(mode.command)
	default:
		return processTypes.MakeExecutionModeSystemLoginShell()
	}
}

func (mode storedMode) MarshalJSON() ([]byte, error) {
	switch mode.kind {
	case "argv":
		args := mode.args
		if args == nil {
			args = []string{}
		}
		return json.Marshal(struct {
			Kind    string   `json:"kind"`
			Program string   `json:"program"`
			Args    []string `json:"args"`
		}{Kind: "argv", Program: mode.program, Args: args})
	case "rcShell":
		return json.Marshal(struct {
			Kind   string `json:"kind"`
			Script string `json:"script"`
		}{Kind: "rcShell", Script: mode.script})
	case "systemShell":
		return json.Marshal(struct {
			Kind    string `json:"kind"`
			Command string `json:"command"`
		}{Kind: "systemShell", Command: mode.command})
	case "systemLoginShell":
		return json.Marshal(struct {
			Kind string `json:"kind"`
		}{Kind: "systemLoginShell"})
	default:
		return nil, fmt.Errorf("invalid stored execution mode")
	}
}

func (mode *storedMode) UnmarshalJSON(data []byte) error {
	var tag struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(data, &tag); err != nil {
		return err
	}
	switch tag.Kind {
	case "argv":
		var value struct {
			Program string   `json:"program"`
			Args    []string `json:"args"`
		}
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*mode = storedMode{kind: "argv", program: value.Program, args: value.Args}
	case "rcShell":
		var value struct {
			Script string `json:"script"`
		}
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*mode = storedMode{kind: "rcShell", script: value.Script}
	case "systemShell":
		var value struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*mode = storedMode{kind: "systemShell", command: value.Command}
	case "systemLoginShell":
		*mode = storedMode{kind: "systemLoginShell"}
	default:
		return fmt.Errorf("invalid stored execution mode")
	}
	return nil
}
