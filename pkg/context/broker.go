package context

import (
	"sort"
	"sync"
	"sync/atomic"
)

type brokerEntry[T any] struct {
	id       uint64
	priority int32
	provider T
}

// Broker manages prioritized providers for a given service interface.
type Broker[T any] struct {
	mu        sync.RWMutex
	nextID    atomic.Uint64
	providers []brokerEntry[T]
}

// ProviderLease represents an active provider registration in a Broker.
type ProviderLease[T any] struct {
	broker *Broker[T]
	id     uint64
	active bool
	mu     sync.Mutex
}

func NewBroker[T any]() *Broker[T] {
	return &Broker[T]{}
}

// Register adds a provider with a given priority (higher priority wins).
func (b *Broker[T]) Register(priority int32, provider T) *ProviderLease[T] {
	id := b.nextID.Add(1)
	b.mu.Lock()
	defer b.mu.Unlock()

	b.providers = append(b.providers, brokerEntry[T]{
		id:       id,
		priority: priority,
		provider: provider,
	})
	// Sort by priority descending, then id ascending
	sort.SliceStable(b.providers, func(i, j int) bool {
		if b.providers[i].priority != b.providers[j].priority {
			return b.providers[i].priority > b.providers[j].priority
		}
		return b.providers[i].id < b.providers[j].id
	})

	return &ProviderLease[T]{
		broker: b,
		id:     id,
		active: true,
	}
}

// Best returns the highest priority active provider, if any.
func (b *Broker[T]) Best() (T, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if len(b.providers) == 0 {
		var zero T
		return zero, false
	}
	return b.providers[0].provider, true
}

// Providers returns a snapshot of all registered providers in priority order.
func (b *Broker[T]) Providers() []T {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]T, len(b.providers))
	for i, entry := range b.providers {
		out[i] = entry.provider
	}
	return out
}

func (b *Broker[T]) remove(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	filtered := b.providers[:0]
	for _, entry := range b.providers {
		if entry.id != id {
			filtered = append(filtered, entry)
		}
	}
	b.providers = filtered
}

// Revoke unregisters the provider from the broker.
func (l *ProviderLease[T]) Revoke() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active {
		l.broker.remove(l.id)
		l.active = false
	}
}
