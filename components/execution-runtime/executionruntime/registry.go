package executionruntime

import "errors"

// registry is the component-local claim table. It rejects duplicate execution
// ids before any native spawn and tracks active leases by lifetime for
// diagnostics.
type registry struct {
	seen   map[string]struct{}
	active [3]int
}

var claims = registry{seen: map[string]struct{}{}}

// claimExecutionID records an execution id for this component instance. An id may never
// be reused, even after the execution finishes, because the durable replay
// filter must not be shadowed by a live duplicate.
func claimExecutionID(id string) error {
	if id == "" {
		return errors.New("execution id is empty")
	}
	if _, ok := claims.seen[id]; ok {
		return errors.New("execution id was already claimed")
	}
	claims.seen[id] = struct{}{}
	return nil
}

func started(kind Kind) [3]int {
	claims.active[kind.index()]++
	return claims.active
}

// finished releases one active lease. The count saturates at zero so a closed
// execution cannot underflow the counter.
func finished(kind Kind) [3]int {
	index := kind.index()
	if claims.active[index] > 0 {
		claims.active[index]--
	}
	return claims.active
}
