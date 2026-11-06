package context

import (
	"errors"
	"testing"
)

func TestBrokerPriorityAndFallback(t *testing.T) {
	broker := NewBroker[string]()

	fallbackLease := broker.Register(10, "fallback")
	preferredLease := broker.Register(20, "preferred")

	best, ok := broker.Best()
	if !ok || best != "preferred" {
		t.Fatalf("expected preferred, got %s (ok=%v)", best, ok)
	}

	preferredLease.Revoke()

	best, ok = broker.Best()
	if !ok || best != "fallback" {
		t.Fatalf("expected fallback, got %s (ok=%v)", best, ok)
	}

	fallbackLease.Revoke()

	_, ok = broker.Best()
	if ok {
		t.Fatal("expected empty broker after all revoked")
	}
}

type testComponent struct {
	name string
	reqs []string
	acts func(ctx *Context, act *Activation) error
}

func (c *testComponent) Name() string           { return c.name }
func (c *testComponent) Requirements() []string { return c.reqs }
func (c *testComponent) Activate(ctx *Context, act *Activation) error {
	if c.acts != nil {
		return c.acts(ctx, act)
	}
	return nil
}

func TestRuntimeDependencyResolutionAndCascade(t *testing.T) {
	ctx := New("root")
	rt := NewRuntime(ctx)

	dep := &testComponent{
		name: "dep",
		acts: func(ctx *Context, act *Activation) error {
			act.ProvideNamed("dep_service", "dep_val")
			return nil
		},
	}

	consumer := &testComponent{
		name: "consumer",
		reqs: []string{"dep_service"},
		acts: func(ctx *Context, act *Activation) error {
			act.ProvideNamed("consumer_service", "consumer_val")
			return nil
		},
	}

	_ = rt.Register(consumer)
	_ = rt.Register(dep)

	if err := rt.Reconcile(); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	st, _ := rt.State("consumer")
	if st != ComponentStateActive {
		t.Fatalf("expected consumer active, got %v", st)
	}

	val, ok := ctx.GetNamed("consumer_service")
	if !ok || val != "consumer_val" {
		t.Fatalf("expected consumer_val, got %v", val)
	}

	// Deactivating dep should cascade deactivate consumer
	rt.deactivate("dep")
	rt.deactivateUnsatisfied()

	st, _ = rt.State("consumer")
	if st != ComponentStateWaiting {
		t.Fatalf("expected consumer waiting after dep revoked, got %v", st)
	}
}

func TestRuntimeRollbackOnActivationFailure(t *testing.T) {
	ctx := New("root")
	rt := NewRuntime(ctx)

	reverted := false
	broken := &testComponent{
		name: "broken",
		acts: func(ctx *Context, act *Activation) error {
			act.Effects.Defer(func() {
				reverted = true
			})
			return errors.New("boom")
		},
	}

	_ = rt.Register(broken)
	if err := rt.Reconcile(); err == nil {
		t.Fatal("expected reconcile error on broken component")
	}

	if !reverted {
		t.Fatal("expected effect scope to be reverted on activation failure")
	}
}
