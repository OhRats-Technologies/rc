package context

type EffectScope struct {
	cleanups []func()
}

func (e *EffectScope) Defer(cleanup func()) {
	e.cleanups = append(e.cleanups, cleanup)
}

func (e *EffectScope) Append(other *EffectScope) {
	if other != nil {
		e.cleanups = append(e.cleanups, other.cleanups...)
	}
}

func (e *EffectScope) IsEmpty() bool {
	return len(e.cleanups) == 0
}

func (e *EffectScope) Revert() {
	for i := len(e.cleanups) - 1; i >= 0; i-- {
		e.cleanups[i]()
	}
	e.cleanups = nil
}
