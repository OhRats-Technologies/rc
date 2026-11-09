package server

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/node"
	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type mcpRunArgs struct {
	DeviceID          string             `json:"deviceId"`
	Argv              []string           `json:"argv"`
	Command           string             `json:"command"`
	Shell             string             `json:"shell"`
	Cwd               string             `json:"cwd"`
	EnvBase           string             `json:"envBase"`
	Env               map[string]*string `json:"env"`
	WaitSeconds       *uint64            `json:"waitSeconds"`
	TimeoutSeconds    *uint64            `json:"timeoutSeconds"`
	MaxRuntimeSeconds *uint64            `json:"maxRuntimeSeconds"`
	Program           string             `json:"program"`
	Args              []string           `json:"args"`
}

func (h *McpHandler) callProcessRun(ctx context.Context, raw json.RawMessage, grant *authstore.OAuthGrant) map[string]any {
	var args mcpRunArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return mcpToolError("invalid process_run arguments")
	}
	deviceID := strings.TrimSpace(args.DeviceID)
	if deviceID == "" {
		return mcpToolError("deviceId is required. Use machines_list to obtain it.")
	}
	if err := h.authorizeDevice(grant.UserID, deviceID, true); err != nil {
		return mcpToolError(err.Error())
	}
	mode, err := executionMode(args)
	if err != nil {
		return mcpToolError(err.Error())
	}
	environment, err := executionEnvironment(args)
	if err != nil {
		return mcpToolError(err.Error())
	}
	wait := uint64(20)
	if args.WaitSeconds != nil {
		wait = *args.WaitSeconds
	} else if args.TimeoutSeconds != nil {
		wait = *args.TimeoutSeconds
	}
	processID := "proc_" + randomID(16)
	spec := node.ProcessSpec{
		ID:          processID,
		Mode:        mode,
		Environment: environment,
		Cwd:         strings.TrimSpace(args.Cwd),
		UserID:      grant.UserID,
		Lifetime:    node.LifetimeManaged,
	}
	if args.MaxRuntimeSeconds != nil && *args.MaxRuntimeSeconds > 0 {
		maxRuntime := *args.MaxRuntimeSeconds * 1000
		spec.MaxRuntimeMs = &maxRuntime
	}
	if !h.local {
		return h.remoteProcessRun(ctx, deviceID, spec, grant, wait)
	}
	if h.exec == nil {
		return mcpToolError("execution control plane is unavailable")
	}
	if spec.MaxRuntimeMs != nil {
		time.AfterFunc(time.Duration(*spec.MaxRuntimeMs)*time.Millisecond, func() {
			_ = h.exec.Signal(processID, "KILL")
		})
	}
	started, err := h.exec.Start(spec)
	if err != nil || !started {
		return mcpToolError("failed to start execution")
	}
	h.bindProcess(processID, grant, deviceID)
	return h.processResult(processID, deviceID, grant, 0, wait, ctx, true)
}

func executionMode(args mcpRunArgs) (protocol.ExecutionMode, error) {
	argv := args.Argv
	if len(argv) == 0 && strings.TrimSpace(args.Program) != "" {
		argv = append([]string{args.Program}, args.Args...)
	}
	hasCommand := strings.TrimSpace(args.Command) != ""
	if len(argv) > 0 && hasCommand {
		return protocol.ExecutionMode{}, mcpArgumentError("exactly one of argv or command is required")
	}
	if len(argv) == 0 && !hasCommand {
		return protocol.ExecutionMode{}, mcpArgumentError("exactly one of argv or command is required")
	}
	if len(argv) > 0 {
		if strings.TrimSpace(argv[0]) == "" {
			return protocol.ExecutionMode{}, mcpArgumentError("argv program must not be empty")
		}
		return protocol.ExecutionMode{
			Kind:    protocol.ExecutionModeArgv,
			Program: argv[0],
			Args:    argv[1:],
		}, nil
	}
	shell := args.Shell
	if shell == "" {
		shell = "rc"
	}
	switch shell {
	case "rc":
		return protocol.ExecutionMode{Kind: protocol.ExecutionModeRcShell, Script: args.Command}, nil
	case "system":
		return protocol.ExecutionMode{Kind: protocol.ExecutionModeSystemShell, Command: args.Command}, nil
	default:
		return protocol.ExecutionMode{}, mcpArgumentError("shell must be rc or system")
	}
}

func executionEnvironment(args mcpRunArgs) (protocol.EnvironmentSpec, error) {
	base := protocol.EnvironmentBaseInherit
	switch args.EnvBase {
	case "", "inherit":
	case "clean":
		base = protocol.EnvironmentBaseClean
	default:
		return protocol.EnvironmentSpec{}, mcpArgumentError("envBase must be inherit or clean")
	}
	changes := make([]protocol.EnvironmentChange, 0, len(args.Env))
	for name, value := range args.Env {
		changes = append(changes, protocol.EnvironmentChange{Name: name, Value: value})
	}
	return protocol.EnvironmentSpec{Base: base, Changes: changes}, nil
}

type mcpArgumentError string

func (err mcpArgumentError) Error() string { return string(err) }
