package executionruntime

import (
	"errors"
	"math"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
)

// Kind is the lifetime class that owns an execution lease.
type Kind uint8

const (
	KindAttached Kind = iota
	KindManaged
	KindScheduled
)

func (k Kind) index() int {
	switch k {
	case KindManaged:
		return 1
	case KindScheduled:
		return 2
	default:
		return 0
	}
}

// Leases carries the deadlines that bound one execution: the reattach grace
// after an attached controller detaches, the requested maximum runtime, and
// the grace window after a terminate signal. Expiry is reported once so the
// runtime kills the group instead of re-killing it on every observation.
type Leases struct {
	kind              Kind
	reattachGraceMs   uint32
	terminateGraceMs  uint32
	attachment        string
	attached          bool
	detachedDeadline  *uint64
	maxDeadline       *uint64
	terminateDeadline *uint64
}

func NewLeases(now uint64, kind Kind, reattachGraceMs uint32, terminateGraceMs uint32, maxRuntimeMs witTypes.Option[uint64]) *Leases {
	leases := &Leases{
		kind:             kind,
		reattachGraceMs:  reattachGraceMs,
		terminateGraceMs: terminateGraceMs,
	}
	if kind == KindAttached {
		deadline := saturatingAdd(now, uint64(reattachGraceMs))
		leases.detachedDeadline = &deadline
	}
	if maxRuntimeMs.IsSome() {
		deadline := saturatingAdd(now, maxRuntimeMs.Some())
		leases.maxDeadline = &deadline
	}
	return leases
}

// Attach binds one controller. Only attached-lifetime executions are
// attachable; managed and scheduled executions have no controller lease.
func (l *Leases) Attach(controller string) error {
	if l.kind != KindAttached {
		return errors.New("execution is not attachable")
	}
	l.attachment = controller
	l.attached = true
	l.detachedDeadline = nil
	return nil
}

// Detach starts the reattach grace only for the controller that held the
// attachment.
func (l *Leases) Detach(controller string, now uint64) {
	if !l.attached || l.attachment != controller {
		return
	}
	l.attachment = ""
	l.attached = false
	deadline := saturatingAdd(now, uint64(l.reattachGraceMs))
	l.detachedDeadline = &deadline
}

// Terminate starts the grace window that follows a terminate signal.
func (l *Leases) Terminate(now uint64) {
	deadline := saturatingAdd(now, uint64(l.terminateGraceMs))
	l.terminateDeadline = &deadline
}

// Expired reports whether any lease deadline has passed and clears the
// deadlines so the caller terminates at most once per expiry.
func (l *Leases) Expired(now uint64) bool {
	expired := false
	if l.maxDeadline != nil && now >= *l.maxDeadline {
		expired = true
	}
	if l.detachedDeadline != nil && now >= *l.detachedDeadline {
		expired = true
	}
	if l.terminateDeadline != nil && now >= *l.terminateDeadline {
		expired = true
	}
	if expired {
		l.maxDeadline = nil
		l.detachedDeadline = nil
		l.terminateDeadline = nil
	}
	return expired
}

func saturatingAdd(value uint64, delta uint64) uint64 {
	if value > math.MaxUint64-delta {
		return math.MaxUint64
	}
	return value + delta
}
