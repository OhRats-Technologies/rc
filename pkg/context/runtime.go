package context

import (
	"fmt"
	"sync"
)

type ComponentState int

const (
	ComponentStateWaiting ComponentState = iota
	ComponentStateActive
)

type activeComponent struct {
	owner      uint64
	services   []string
	activation Activation
}

type Runtime struct {
	mu         sync.Mutex
	context    *Context
	components []Component
	active     map[string]*activeComponent
}

func NewRuntime(ctx *Context) *Runtime {
	return &Runtime{
		context: ctx,
		active:  make(map[string]*activeComponent),
	}
}

func (r *Runtime) Context() *Context {
	return r.context
}

func (r *Runtime) Register(c Component) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.components {
		if existing.Name() == c.Name() {
			return fmt.Errorf("component %s is already registered", c.Name())
		}
	}
	r.components = append(r.components, c)
	return nil
}

func (r *Runtime) State(name string) (ComponentState, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.components {
		if c.Name() == name {
			if _, ok := r.active[name]; ok {
				return ComponentStateActive, true
			}
			return ComponentStateWaiting, true
		}
	}
	return ComponentStateWaiting, false
}

func (r *Runtime) Reconcile() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.deactivateUnsatisfied()
	for {
		progress := false
		for _, comp := range r.components {
			if _, isActive := r.active[comp.Name()]; isActive {
				continue
			}
			if !r.requirementsAvailable(comp) {
				continue
			}
			if err := r.activate(comp); err != nil {
				return err
			}
			progress = true
		}
		if !progress {
			break
		}
	}
	return nil
}

func (r *Runtime) activate(comp Component) error {
	var act Activation
	if err := comp.Activate(r.context, &act); err != nil {
		act.Effects.Revert()
		return fmt.Errorf("component %s failed to activate: %w", comp.Name(), err)
	}

	for _, s := range act.Services {
		if r.context.ContainsLocal(s.Name) {
			act.Effects.Revert()
			return fmt.Errorf("component %s attempted to replace service %s", comp.Name(), s.Name)
		}
	}

	owner := nextOwnerID.Add(1)
	var inserted []string
	for _, s := range act.Services {
		r.context.mu.Lock()
		r.context.services[s.Name] = serviceEntry{
			owner: owner,
			value: s.Value,
		}
		r.context.mu.Unlock()
		inserted = append(inserted, s.Name)
	}

	r.active[comp.Name()] = &activeComponent{
		owner:      owner,
		services:   inserted,
		activation: act,
	}
	return nil
}

func (r *Runtime) deactivateUnsatisfied() {
	for {
		var toDeactivate []string
		for i := len(r.components) - 1; i >= 0; i-- {
			comp := r.components[i]
			if _, isActive := r.active[comp.Name()]; isActive && !r.requirementsAvailable(comp) {
				toDeactivate = append(toDeactivate, comp.Name())
			}
		}
		if len(toDeactivate) == 0 {
			break
		}
		for _, name := range toDeactivate {
			r.deactivate(name)
		}
	}
}

func (r *Runtime) deactivate(name string) {
	active, ok := r.active[name]
	if !ok {
		return
	}
	delete(r.active, name)
	for i := len(active.services) - 1; i >= 0; i-- {
		r.context.removeRaw(active.services[i], active.owner)
	}
	active.activation.Effects.Revert()
}

func (r *Runtime) requirementsAvailable(comp Component) bool {
	for _, req := range comp.Requirements() {
		if !r.context.Contains(req) {
			return false
		}
	}
	return true
}

func (r *Runtime) Shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.components) - 1; i >= 0; i-- {
		r.deactivate(r.components[i].Name())
	}
}
