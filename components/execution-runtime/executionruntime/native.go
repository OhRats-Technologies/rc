package executionruntime

import (
	"errors"

	processHost "wit_component/ohrats_rc_process_process_host"
	processRuntime "wit_component/ohrats_rc_process_runtime"
	processTypes "wit_component/ohrats_rc_process_types"
	shellExecutor "wit_component/ohrats_rc_shell_executor"
)

// output is one stream read returned by a native poll.
type output struct {
	Kind  processTypes.StreamKind
	Bytes []uint8
}

type nativeKind uint8

const (
	nativeProcess nativeKind = iota
	nativeShell
	nativeFailed
)

// native dispatches one execution to the host process group or the portable RC
// Shell executor. A failed start stays an error state so every later call
// reports the original failure instead of spawning again.
type native struct {
	kind    nativeKind
	process *hostProcess
	shellID string
	failure string
}

func startNative(id string, plan processTypes.StartPlan) *native {
	started, err := spawnNative(id, plan)
	if err != nil {
		return &native{kind: nativeFailed, failure: err.Error()}
	}
	return started
}

func spawnNative(id string, plan processTypes.StartPlan) (*native, error) {
	if plan.Mode.Tag() == processTypes.ExecutionMode_RcShell {
		started := shellExecutor.Start(id, shellExecutor.StartRequest{
			Script:      plan.Mode.RcShell(),
			Cwd:         plan.Cwd,
			Environment: plan.Environment,
			Terminal:    plan.Terminal,
		})
		if started.IsErr() {
			return nil, errors.New(started.Err())
		}
		return &native{kind: nativeShell, shellID: id}, nil
	}
	request, err := spawnRequest(plan)
	if err != nil {
		return nil, err
	}
	group, err := createGroup(id)
	if err != nil {
		return nil, err
	}
	spawned := group.Spawn(request)
	if spawned.IsErr() {
		group.Drop()
		return nil, errors.New(spawned.Err())
	}
	value := spawned.Ok()
	process := &hostProcess{
		group:     group,
		child:     value.Child,
		stdout:    value.Stdout,
		stdoutEOF: false,
		stderrEOF: value.Stderr.IsNone(),
	}
	if value.Stdin.IsSome() {
		process.stdin = value.Stdin.Some()
	}
	if value.Stderr.IsSome() {
		process.stderr = value.Stderr.Some()
	}
	return &native{kind: nativeProcess, process: process}, nil
}

func createGroup(id string) (*processHost.ExecutionGroup, error) {
	group := processHost.CreateGroup(id)
	if group.IsErr() {
		return nil, errors.New(group.Err())
	}
	return group.Ok(), nil
}

// Poll drains up to budget bytes and reports the exit only once every stream
// has reached EOF, so bytes the OS already accepted are never lost.
func (n *native) Poll(budget uint32) ([]output, *processRuntime.ExitResult, error) {
	switch n.kind {
	case nativeFailed:
		return nil, nil, errors.New(n.failure)
	case nativeShell:
		polled := shellExecutor.Poll(n.shellID, budget)
		if polled.IsErr() {
			return nil, nil, errors.New(polled.Err())
		}
		result := polled.Ok()
		read := make([]output, 0, len(result.Output))
		for _, value := range result.Output {
			read = append(read, output{Kind: value.Kind, Bytes: value.Bytes})
		}
		// An intermediate pipeline stage may have exited while the complete
		// script still has output to drain or later stages to run.
		if result.State != shellExecutor.State_Exited || result.Exit.IsNone() {
			return read, nil, nil
		}
		exit := result.Exit.Some()
		return read, &processRuntime.ExitResult{Code: exit.Code, Signal: exit.Signal}, nil
	default:
		return n.process.poll(budget)
	}
}

func (n *native) Input(bytes []uint8) (uint32, error) {
	switch n.kind {
	case nativeFailed:
		return 0, errors.New(n.failure)
	case nativeShell:
		written := shellExecutor.Input(n.shellID, bytes)
		if written.IsErr() {
			return 0, errors.New(written.Err())
		}
		return written.Ok(), nil
	default:
		return n.process.input(bytes)
	}
}

func (n *native) CloseInput() error {
	switch n.kind {
	case nativeFailed:
		return errors.New(n.failure)
	case nativeShell:
		closed := shellExecutor.CloseInput(n.shellID)
		if closed.IsErr() {
			return errors.New(closed.Err())
		}
		return nil
	default:
		return n.process.closeInput()
	}
}

func (n *native) Resize(cols uint16, rows uint16) error {
	switch n.kind {
	case nativeFailed:
		return errors.New(n.failure)
	case nativeShell:
		resized := shellExecutor.Resize(n.shellID, cols, rows)
		if resized.IsErr() {
			return errors.New(resized.Err())
		}
		return nil
	default:
		group, err := n.process.executionGroup()
		if err != nil {
			return err
		}
		resized := group.Resize(cols, rows)
		if resized.IsErr() {
			return errors.New(resized.Err())
		}
		return nil
	}
}

func (n *native) Signal(signal processTypes.Signal) error {
	switch n.kind {
	case nativeFailed:
		return errors.New(n.failure)
	case nativeShell:
		requested := shellExecutor.RequestSignal(n.shellID, signal)
		if requested.IsErr() {
			return errors.New(requested.Err())
		}
		return nil
	default:
		group, err := n.process.executionGroup()
		if err != nil {
			return err
		}
		signalled := group.Signal(signal)
		if signalled.IsErr() {
			return errors.New(signalled.Err())
		}
		return nil
	}
}

// Close releases the native execution. It is idempotent because completion,
// explicit close, and resource drop may all run it.
func (n *native) Close() {
	switch n.kind {
	case nativeFailed:
		return
	case nativeShell:
		shellExecutor.Close(n.shellID)
	default:
		n.process.closeGroup()
	}
}
