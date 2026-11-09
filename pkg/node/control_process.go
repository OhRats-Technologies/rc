package node

import (
	"encoding/base64"
	"errors"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

func (m *ControlManager) dispatchControlMessage(session *ControlSession, msg *protocol.ControlMessage) ([]byte, error) {
	switch msg.Type {
	case protocol.TypeProcessStart:
		return nil, m.startControlledProcess(session, msg)
	case protocol.TypeProcessAttach:
		if !session.CanExecute || m.processes == nil ||
			!m.processes.AttachSecure(msg.ID, session.SessionID) {
			return nil, errors.New("process not found")
		}
		go m.pumpProcess(session.SessionID, msg.ID)
	case protocol.TypeProcessStdin:
		if !session.CanExecute {
			return nil, errors.New("execution permission required")
		}
		if m.processes == nil {
			return nil, nil
		}
		data, err := base64.RawURLEncoding.DecodeString(msg.Data)
		if err != nil || len(data) > protocol.ProcessInputChunkLimit {
			return nil, errors.New("invalid process input")
		}
		if err := m.processes.Input(msg.ID, data); err != nil {
			return nil, err
		}
	case protocol.TypeProcessStdinClose:
		if !session.CanExecute {
			return nil, errors.New("execution permission required")
		}
		if m.processes == nil {
			return nil, nil
		}
		if err := m.processes.CloseInput(msg.ID); err != nil {
			return nil, err
		}
	case protocol.TypeProcessResize:
		if !session.CanExecute {
			return nil, errors.New("execution permission required")
		}
		if m.processes == nil {
			return nil, nil
		}
		return nil, m.processes.Resize(msg.ID, msg.Cols, msg.Rows)
	case protocol.TypeProcessSignal:
		if !session.CanExecute {
			return nil, errors.New("execution permission required")
		}
		if m.processes == nil {
			return nil, nil
		}
		return nil, m.processes.Signal(msg.ID, msg.Signal)
	case protocol.TypeProcessStdout, protocol.TypeProcessStderr,
		protocol.TypeProcessStarted, protocol.TypeProcessExit,
		protocol.TypeNodeUpdate, protocol.TypeControlResult, protocol.TypeControlRevoked:
		return nil, errors.New("invalid client control message")
	default:
		return nil, errors.New("unsupported control message")
	}
	return nil, nil
}

func (m *ControlManager) startControlledProcess(session *ControlSession, msg *protocol.ControlMessage) error {
	if !session.CanExecute || m.processes == nil {
		return errors.New("execution permission required")
	}
	if msg.Mode == nil || msg.ID == "" {
		return errors.New("invalid process start")
	}
	environment := protocol.EnvironmentSpec{Base: protocol.EnvironmentBaseInherit}
	if msg.Environment != nil {
		environment = *msg.Environment
	}
	cwd := ""
	if msg.Cwd != nil {
		cwd = *msg.Cwd
	}
	started, err := m.processes.Start(ProcessSpec{
		ID: msg.ID, Mode: *msg.Mode, Environment: environment, Cwd: cwd,
		Terminal: msg.Terminal, SessionID: session.SessionID,
		UserID: session.UserID, Lifetime: LifetimeAttached,
	})
	if err != nil || !started {
		m.sendFrame(session.SessionID, protocol.ControlMessage{
			Type: protocol.TypeProcessExit, ID: msg.ID,
			Error: "process could not be started",
		})
		return nil
	}
	m.processes.AttachSecure(msg.ID, session.SessionID)
	m.sendFrame(session.SessionID, protocol.ControlMessage{
		Type: protocol.TypeProcessStarted, ID: msg.ID,
	})
	go m.pumpProcess(session.SessionID, msg.ID)
	return nil
}

func (m *ControlManager) pumpProcess(sessionID, processID string) {
	cursor := uint64(0)
	for {
		read, ok := m.processes.ExecutionRead(processID, cursor, 256*1024)
		if !ok {
			return
		}
		for _, chunk := range read.Chunks {
			kind := protocol.TypeProcessStdout
			if chunk.Stream == StreamStderr {
				kind = protocol.TypeProcessStderr
			}
			if !m.sendFrame(sessionID, protocol.ControlMessage{
				Type: kind, ID: processID,
				Data: base64.RawURLEncoding.EncodeToString(chunk.Bytes),
			}) {
				return
			}
			cursor = chunk.Cursor
		}
		if read.Status != "running" {
			exit := int32(0)
			if read.ExitCode != nil {
				exit = *read.ExitCode
			}
			m.sendFrame(sessionID, protocol.ControlMessage{
				Type: protocol.TypeProcessExit, ID: processID,
				ExitCode: &exit, Signal: read.Signal,
			})
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}
