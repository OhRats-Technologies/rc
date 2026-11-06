package executionruntime

import (
	"errors"

	processHost "wit_component/ohrats_rc_process_process_host"
	processRuntime "wit_component/ohrats_rc_process_runtime"
	processTypes "wit_component/ohrats_rc_process_types"
)

type hostProcess struct {
	group     *processHost.ExecutionGroup
	child     *processHost.Child
	stdin     *processHost.ByteStream
	stdout    *processHost.ByteStream
	stderr    *processHost.ByteStream
	stdoutEOF bool
	stderrEOF bool
	exit      *processRuntime.ExitResult
}

func (p *hostProcess) poll(budget uint32) ([]output, *processRuntime.ExitResult, error) {
	var read []output
	if !p.stdoutEOF {
		eof, err := readStream(p.stdout, processTypes.StreamKind_Stdout, budget, &read)
		if err != nil {
			return nil, nil, err
		}
		p.stdoutEOF = eof
	}
	if !p.stderrEOF && p.stderr != nil {
		eof, err := readStream(p.stderr, processTypes.StreamKind_Stderr, budget, &read)
		if err != nil {
			return nil, nil, err
		}
		p.stderrEOF = eof
	}
	if p.exit == nil {
		polled := p.child.PollExit()
		if polled.IsErr() {
			return nil, nil, errors.New(polled.Err())
		}
		if polled.Ok().IsSome() {
			value := polled.Ok().Some()
			p.exit = &processRuntime.ExitResult{Code: value.Code, Signal: value.Signal}
			// Close descendants and the terminal producer, then keep draining
			// bytes the OS already accepted before publishing the final exit.
			p.closeGroup()
		}
	}
	if p.stdoutEOF && p.stderrEOF {
		return read, p.exit, nil
	}
	return read, nil, nil
}

func (p *hostProcess) input(bytes []uint8) (uint32, error) {
	if p.stdin == nil {
		return 0, errors.New("execution stdin is closed")
	}
	written := p.stdin.Write(bytes)
	if written.IsErr() {
		return 0, errors.New(written.Err())
	}
	if written.Ok().Tag() == processHost.WriteResult_Accepted {
		return written.Ok().Accepted(), nil
	}
	return 0, nil
}

func (p *hostProcess) closeInput() error {
	stdin := p.stdin
	if stdin == nil {
		return nil
	}
	p.stdin = nil
	closed := stdin.CloseWrite()
	stdin.Drop()
	if closed.IsErr() {
		return errors.New(closed.Err())
	}
	return nil
}

func (p *hostProcess) executionGroup() (*processHost.ExecutionGroup, error) {
	if p.group == nil {
		return nil, errors.New("execution is closed")
	}
	return p.group, nil
}

func (p *hostProcess) closeGroup() {
	if p.group == nil {
		return
	}
	p.group.Close()
	p.group.Drop()
	p.group = nil
}

func readStream(stream *processHost.ByteStream, kind processTypes.StreamKind, budget uint32, read *[]output) (bool, error) {
	value := stream.Read(budget)
	if value.IsErr() {
		return false, errors.New(value.Err())
	}
	switch value.Ok().Tag() {
	case processHost.ReadResult_Data:
		*read = append(*read, output{Kind: kind, Bytes: value.Ok().Data()})
		return false, nil
	case processHost.ReadResult_Eof:
		return true, nil
	default:
		return false, nil
	}
}
