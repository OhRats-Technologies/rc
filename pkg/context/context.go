package context

import (
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
)

var nextOwnerID atomic.Uint64

type ServiceKey struct {
	Name string
	Type reflect.Type
}

func ServiceKeyOf[T any]() ServiceKey {
	var zero T
	t := reflect.TypeOf(&zero).Elem()
	return ServiceKey{
		Name: t.String(),
		Type: t,
	}
}

func ServiceKeyNamed(name string) ServiceKey {
	return ServiceKey{Name: name}
}

type serviceEntry struct {
	owner uint64
	value any
}

type Context struct {
	realm    string
	parent   *Context
	mu       sync.RWMutex
	services map[string]serviceEntry
}

type ServiceLease struct {
	context *Context
	key     string
	owner   uint64
	active  bool
	mu      sync.Mutex
}

func New(realm string) *Context {
	return &Context{
		realm:    realm,
		services: make(map[string]serviceEntry),
	}
}

func (c *Context) Child(realm string) *Context {
	return &Context{
		realm:    realm,
		parent:   c,
		services: make(map[string]serviceEntry),
	}
}

func (c *Context) Realm() string {
	return c.realm
}

func (c *Context) ProvideNamed(name string, value any) (*ServiceLease, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.services[name]; exists {
		return nil, fmt.Errorf("service %s is already provided in realm %s", name, c.realm)
	}

	owner := nextOwnerID.Add(1)
	c.services[name] = serviceEntry{
		owner: owner,
		value: value,
	}

	return &ServiceLease{
		context: c,
		key:     name,
		owner:   owner,
		active:  true,
	}, nil
}

func (c *Context) GetNamed(name string) (any, bool) {
	c.mu.RLock()
	entry, ok := c.services[name]
	c.mu.RUnlock()

	if ok {
		return entry.value, true
	}
	if c.parent != nil {
		return c.parent.GetNamed(name)
	}
	return nil, false
}

func (c *Context) Contains(name string) bool {
	_, ok := c.GetNamed(name)
	return ok
}

func (c *Context) ContainsLocal(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.services[name]
	return ok
}

func (c *Context) removeRaw(name string, owner uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.services[name]; ok && entry.owner == owner {
		delete(c.services, name)
		return true
	}
	return false
}

func (l *ServiceLease) Revoke() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active {
		l.context.removeRaw(l.key, l.owner)
		l.active = false
	}
}
