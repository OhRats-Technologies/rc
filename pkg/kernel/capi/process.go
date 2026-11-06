//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	resourceGroup  uint32 = 1
	resourceStream uint32 = 2
	resourceChild  uint32 = 3
)

type processGroup struct {
	id     string
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	done   bool
}

type streamResource struct {
	mu     sync.Mutex
	reader io.Reader
	writer io.WriteCloser
	closed bool
}

type childResource struct {
	group *processGroup
}

var (
	resourceRegistry sync.Map
	resourceSequence atomic.Uint32
)

func addResourceObject(object any) uint32 {
	rep := resourceSequence.Add(1)
	resourceRegistry.Store(rep, object)
	return rep
}

func lookupResource(rep uint32) any {
	value, _ := resourceRegistry.Load(rep)
	return value
}

func releaseResource(rep uint32) {
	value, ok := resourceRegistry.LoadAndDelete(rep)
	if !ok {
		return
	}
	switch object := value.(type) {
	case *processGroup:
		object.close()
	case *streamResource:
		object.close()
	case *timerResource:
		releaseTimer(object)
	case *netConnection:
		releaseConnection(object)
	}
}

func (g *processGroup) close() {
	g.mu.Lock()
	cmd := g.cmd
	streams := []io.Closer{}
	if g.stdin != nil {
		streams = append(streams, g.stdin)
	}
	if g.stdout != nil {
		streams = append(streams, g.stdout)
	}
	if g.stderr != nil {
		streams = append(streams, g.stderr)
	}
	g.mu.Unlock()
	for _, stream := range streams {
		_ = stream.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
}

func (s *streamResource) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	if s.writer != nil {
		_ = s.writer.Close()
	}
	if closer, ok := s.reader.(io.Closer); ok {
		_ = closer.Close()
	}
}

func (s *streamResource) read(max int) ([]byte, bool, bool) {
	s.mu.Lock()
	reader := s.reader
	s.mu.Unlock()
	if reader == nil || max < 1 {
		return nil, false, true
	}
	buffer := make([]byte, max)
	n, err := reader.Read(buffer)
	if n > 0 {
		return buffer[:n], false, false
	}
	if err == io.EOF {
		return nil, true, false
	}
	return nil, false, false
}

func processCreateGroup(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) {
	if nargs < 1 || nresults < 1 {
		setStringResult(results, false, "invalid create-group request")
		return
	}
	group := &processGroup{id: valueString(argValue(args, 0))}
	rep := addResourceObject(group)
	resource, err := hostResource(ctx, rep, resourceGroup)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	payload := newValue()
	C.rc_set_resource(payload, resource)
	C.rc_set_result(results, true, payload)
}

func processGroupSpawn(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) {
	group, ok := selfGroup(ctx, args)
	if !ok || nargs < 2 || nresults < 1 {
		setStringResult(results, false, "invalid spawn request")
		return
	}
	request := argValue(args, 1)
	program := valueString(recordField(request, "program"))
	var argv []string
	if list := recordField(request, "args"); list != nil {
		for i := 0; i < int(C.rc_list_len(list)); i++ {
			argv = append(argv, valueString(C.rc_list_get(list, C.size_t(i))))
		}
	}
	command := exec.Command(program, argv...)
	if cwd := recordField(request, "cwd"); cwd != nil {
		if option := C.rc_option_get(cwd); option != nil {
			command.Dir = valueString(option)
		}
	}
	command.Env = processEnvironment(recordField(request, "environment"))
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := command.StdinPipe()
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	if err := command.Start(); err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	group.mu.Lock()
	group.cmd = command
	group.stdin = stdin
	group.stdout = stdout
	group.stderr = stderr
	group.mu.Unlock()
	go func() {
		_ = command.Wait()
		group.mu.Lock()
		group.done = true
		group.mu.Unlock()
	}()
	childRep := addResourceObject(&childResource{group: group})
	stdinRep := addResourceObject(&streamResource{writer: stdin})
	stdoutRep := addResourceObject(&streamResource{reader: stdout})
	stderrRep := addResourceObject(&streamResource{reader: stderr})

	childResourceValue, err := hostResource(ctx, childRep, resourceChild)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	stdinResourceValue, err := hostResource(ctx, stdinRep, resourceStream)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	stdoutResourceValue, err := hostResource(ctx, stdoutRep, resourceStream)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	stderrResourceValue, err := hostResource(ctx, stderrRep, resourceStream)
	if err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	spawned := newValue()
	C.rc_alloc_record(spawned, 4)
	C.rc_set_resource(C.rc_record_item(spawned, 0, cString("child"), 5), childResourceValue)
	stdinOption := newValue()
	C.rc_set_some_option(stdinOption, resourceValue(stdinResourceValue))
	*C.rc_record_item(spawned, 1, cString("stdin"), 5) = *stdinOption
	C.rc_set_resource(C.rc_record_item(spawned, 2, cString("stdout"), 6), stdoutResourceValue)
	stderrOption := newValue()
	C.rc_set_some_option(stderrOption, resourceValue(stderrResourceValue))
	*C.rc_record_item(spawned, 3, cString("stderr"), 6) = *stderrOption
	C.rc_set_result(results, true, spawned)
}

func processGroupSignal(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) {
	group, ok := selfGroup(ctx, args)
	if !ok || nargs < 2 {
		setStringResult(results, false, "invalid signal request")
		return
	}
	name := valueEnum(argValue(args, 1))
	group.mu.Lock()
	cmd := group.cmd
	group.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		setStringResult(results, false, "process has not started")
		return
	}
	var signal os.Signal = os.Interrupt
	switch name {
	case "terminate":
		signal = syscall.SIGTERM
	case "kill":
		signal = syscall.SIGKILL
	}
	if err := cmd.Process.Signal(signal); err != nil {
		setStringResult(results, false, err.Error())
		return
	}
	C.rc_set_result(results, true, nil)
}

func processGroupResize(_ unsafe.Pointer, _ *C.wasmtime_component_val_t, _ C.size_t, results *C.wasmtime_component_val_t, _ C.size_t) {
	C.rc_set_result(results, true, nil)
}

func processGroupClose(ctx unsafe.Pointer, args *C.wasmtime_component_val_t, _ C.size_t, _ *C.wasmtime_component_val_t, _ C.size_t) {
	if group, ok := selfGroup(ctx, args); ok {
		group.close()
	}
}
