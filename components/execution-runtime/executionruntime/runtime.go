package executionruntime

import (
	"errors"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	clockHost "wit_component/ohrats_rc_process_clock_host"
	processPolicy "wit_component/ohrats_rc_process_policy"
	processRuntime "wit_component/ohrats_rc_process_runtime"
	processTypes "wit_component/ohrats_rc_process_types"
)

// State is the component-local state behind one exported execution resource.
// It owns the native execution, the bounded output journal, the lease
// deadlines, and the authorization identity captured at start.
type State struct {
	id          string
	ownerUserID string
	principal   processTypes.Principal
	native      *native
	journal     *journal
	exit        *processRuntime.ExitResult
	leases      *Leases
	leaseKind   Kind
	registered  bool
}

// Start authorizes a start request, claims its id in the component registry and
// the durable replay filter, and then spawns the native execution. A failed
// native start still returns an execution whose reads report the failure.
func Start(request processTypes.StartRequest) (*State, error) {
	plan := processPolicy.AuthorizeStart(request)
	if plan.IsErr() {
		return nil, errors.New(plan.Err())
	}
	authorized := plan.Ok()
	if err := claimExecutionID(request.ExecutionId); err != nil {
		return nil, err
	}
	if err := claimReplay(request.ExecutionId); err != nil {
		return nil, err
	}
	kind := leaseKind(request.Lifetime)
	state := &State{
		id:          request.ExecutionId,
		ownerUserID: request.Principal.UserId,
		principal:   request.Principal,
		journal:     newJournal(authorized.ScrollbackBytes),
		leases: NewLeases(
			clockHost.NowUnixMs(),
			kind,
			authorized.ReattachGraceMs,
			authorized.TerminateGraceMs,
			authorized.MaxRuntimeMs,
		),
		leaseKind:  kind,
		registered: true,
	}
	state.native = startNative(state.id, authorized)
	counts(started(state.leaseKind))
	return state, nil
}

func leaseKind(lifetime processTypes.Lifetime) Kind {
	switch lifetime {
	case processTypes.Lifetime_Managed:
		return KindManaged
	case processTypes.Lifetime_Scheduled:
		return KindScheduled
	default:
		return KindAttached
	}
}

func (s *State) ID() string {
	return s.id
}

// State enforces lease expiry and completion before reporting the state.
func (s *State) State() processRuntime.ExecutionState {
	s.enforceLeases()
	s.pollExit()
	if s.exit != nil {
		return processRuntime.ExecutionState_Exited
	}
	return processRuntime.ExecutionState_Running
}

// Read drains bounded output, reports completion, and returns journal chunks
// from the requested cursor.
func (s *State) Read(cursor uint64, maxBytes uint32) (processRuntime.ReadResult, error) {
	s.enforceLeases()
	budget := maxBytes
	if budget < 1 {
		budget = 1
	}
	s.drain(budget)
	s.pollExit()
	chunks, next, more := s.journal.read(cursor, uint64(maxBytes))
	return processRuntime.ReadResult{
		State:           s.executionState(),
		Chunks:          chunks,
		NextCursor:      next,
		TruncatedBefore: s.journal.truncatedBefore(),
		More:            more,
		Exit:            optionExit(s.exit),
	}, nil
}

func (s *State) Input(bytes []uint8) (uint32, error) {
	if err := s.authorizeAccess(processTypes.Action_Input); err != nil {
		return 0, err
	}
	return s.native.Input(bytes)
}

func (s *State) CloseInput() error {
	if err := s.authorizeAccess(processTypes.Action_CloseInput); err != nil {
		return err
	}
	return s.native.CloseInput()
}

// Attach binds one controller id to an attached-lifetime execution.
func (s *State) Attach(controllerID string) error {
	if controllerID == "" {
		return errors.New("controller id is empty")
	}
	if err := s.authorizeAccess(processTypes.Action_Attach); err != nil {
		return err
	}
	return s.leases.Attach(controllerID)
}

// Detach releases the attachment and starts the reattach grace window.
func (s *State) Detach(controllerID string) error {
	if err := s.authorizeAccess(processTypes.Action_Attach); err != nil {
		return err
	}
	s.leases.Detach(controllerID, clockHost.NowUnixMs())
	return nil
}

func (s *State) Resize(size processTypes.TerminalSize) error {
	normalized := processPolicy.NormalizeResize(processTypes.ResizeRequest{
		Access: s.accessRequest(processTypes.Action_Resize),
		Cols:   size.Cols,
		Rows:   size.Rows,
	})
	if normalized.IsErr() {
		return errors.New(normalized.Err())
	}
	resolved := normalized.Ok()
	return s.native.Resize(resolved.Cols, resolved.Rows)
}

// Signal authorizes the signal first. A terminate signal also starts the
// policy-owned grace window before escalation.
func (s *State) Signal(signal processTypes.Signal) error {
	authorized := processPolicy.AuthorizeSignal(processTypes.SignalRequest{
		Access: s.accessRequest(processTypes.Action_Signal),
		Signal: signal,
	})
	if authorized.IsErr() {
		return errors.New(authorized.Err())
	}
	effective := authorized.Ok()
	if effective == processTypes.Signal_Terminate {
		s.leases.Terminate(clockHost.NowUnixMs())
	}
	return s.native.Signal(effective)
}

// Close releases the native execution without unregistering it.
func (s *State) Close() {
	s.native.Close()
}

// Drop is the resource destructor: close the native execution and release the
// active lease registration exactly once.
func (s *State) Drop() {
	s.native.Close()
	s.finishRegistration()
}

func (s *State) executionState() processRuntime.ExecutionState {
	if s.exit != nil {
		return processRuntime.ExecutionState_Exited
	}
	return processRuntime.ExecutionState_Running
}

func optionExit(exit *processRuntime.ExitResult) witTypes.Option[processRuntime.ExitResult] {
	if exit == nil {
		return witTypes.None[processRuntime.ExitResult]()
	}
	return witTypes.Some(*exit)
}

func (s *State) accessRequest(action processTypes.Action) processTypes.AccessRequest {
	return processTypes.AccessRequest{
		ExecutionId: s.id,
		OwnerUserId: s.ownerUserID,
		Action:      action,
		Principal:   s.principal,
	}
}

func (s *State) authorizeAccess(action processTypes.Action) error {
	authorized := processPolicy.AuthorizeAccess(s.accessRequest(action))
	if authorized.IsErr() {
		return errors.New(authorized.Err())
	}
	return nil
}

// drain moves native output into the journal. A native failure becomes a
// bounded stderr diagnostic and exit code 1 rather than an unbounded error.
func (s *State) drain(budget uint32) {
	if s.exit != nil {
		return
	}
	read, exit, err := s.native.Poll(budget)
	if err != nil {
		s.journal.push(processTypes.StreamKind_Stderr, []byte("RC execution failed: "+err.Error()+"\n"))
		s.exit = &processRuntime.ExitResult{
			Code:   witTypes.Some[uint32](1),
			Signal: witTypes.None[processTypes.Signal](),
		}
		s.finishRegistration()
		return
	}
	for _, value := range read {
		s.journal.push(value.Kind, value.Bytes)
	}
	if s.exit == nil && exit != nil {
		s.exit = exit
		s.finishRegistration()
	}
}

func (s *State) pollExit() {
	if s.exit != nil {
		return
	}
	s.drain(1)
}

// finishRegistration closes the native execution and releases the active lease
// count exactly once per execution.
func (s *State) finishRegistration() {
	if !s.registered {
		return
	}
	s.registered = false
	s.native.Close()
	counts(finished(s.leaseKind))
}

// enforceLeases kills the execution group once a lease deadline passes.
func (s *State) enforceLeases() {
	if s.leases.Expired(clockHost.NowUnixMs()) {
		_ = s.native.Signal(processTypes.Signal_Kill)
	}
}
