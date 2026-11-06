package mesh

import (
	"errors"
	"sort"
	"sync"
	"sync/atomic"
)

var (
	ErrRouteUnavailable  = errors.New("no mesh route is available")
	ErrRouteRejected     = errors.New("mesh route rejected the envelope")
	ErrRouteDisconnected = errors.New("mesh route disconnected")
)

type RouteProvider interface {
	Send(envelope *MeshEnvelope) error
}

type routeKey struct {
	Realm  RealmID
	Target RouteTarget
}

type routeEntry struct {
	id         uint64
	descriptor RouteDescriptor
	provider   RouteProvider
}

type RouteBroker struct {
	mu     sync.RWMutex
	nextID atomic.Uint64
	routes map[routeKey][]routeEntry
}

func NewRouteBroker() *RouteBroker {
	return &RouteBroker{
		routes: make(map[routeKey][]routeEntry),
	}
}

type RouteLease struct {
	broker *RouteBroker
	key    routeKey
	id     uint64
	active bool
	mu     sync.Mutex
}

func (b *RouteBroker) Register(descriptor RouteDescriptor, provider RouteProvider) *RouteLease {
	id := b.nextID.Add(1)
	key := routeKey{
		Realm:  descriptor.Realm,
		Target: descriptor.Target,
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	entries := append(b.routes[key], routeEntry{
		id:         id,
		descriptor: descriptor,
		provider:   provider,
	})
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].descriptor.Cost != entries[j].descriptor.Cost {
			return entries[i].descriptor.Cost < entries[j].descriptor.Cost
		}
		return entries[i].id < entries[j].id
	})
	b.routes[key] = entries

	return &RouteLease{
		broker: b,
		key:    key,
		id:     id,
		active: true,
	}
}

func (b *RouteBroker) Send(target *RouteTarget, envelope *MeshEnvelope, nowMs int64) error {
	key := routeKey{
		Realm:  envelope.Realm,
		Target: *target,
	}

	b.mu.RLock()
	entries := append([]routeEntry(nil), b.routes[key]...)
	b.mu.RUnlock()

	attempted := false
	for _, entry := range entries {
		if entry.descriptor.ExpiresAt != 0 && entry.descriptor.ExpiresAt <= nowMs {
			continue
		}
		attempted = true
		if err := entry.provider.Send(envelope); err == nil {
			return nil
		}
	}

	if attempted {
		return ErrRouteDisconnected
	}
	return ErrRouteUnavailable
}

func (l *RouteLease) Revoke() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.active {
		return
	}
	l.broker.mu.Lock()
	defer l.broker.mu.Unlock()

	entries := l.broker.routes[l.key]
	filtered := entries[:0]
	for _, e := range entries {
		if e.id != l.id {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == 0 {
		delete(l.broker.routes, l.key)
	} else {
		l.broker.routes[l.key] = filtered
	}
	l.active = false
}
