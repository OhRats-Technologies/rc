package mesh

import (
	"container/heap"
	"errors"
	"sync"
)

var (
	ErrTopologyRealm     = errors.New("mesh advertisement belongs to another realm")
	ErrTopologyUntrusted = errors.New("mesh peer is not trusted in this realm")
)

type Route struct {
	Destination PeerID   `json:"destination"`
	NextHop     PeerID   `json:"nextHop"`
	TotalCost   uint64   `json:"totalCost"`
	Path        []PeerID `json:"path"`
}

type ServiceRoute struct {
	Service   string `json:"service"`
	Provider  PeerID `json:"provider"`
	Route     Route  `json:"route"`
	TotalCost uint64 `json:"totalCost"`
}

type Topology struct {
	mu             sync.RWMutex
	realm          RealmID
	local          PeerID
	keys           map[PeerID]string
	advertisements map[PeerID]SignedAdvertisement
}

func NewTopology(realm RealmID, local PeerID) *Topology {
	return &Topology{
		realm:          realm,
		local:          local,
		keys:           make(map[PeerID]string),
		advertisements: make(map[PeerID]SignedAdvertisement),
	}
}

func (t *Topology) TrustPeer(peer PeerID, publicKey string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.keys[peer] = publicKey
}

func (t *Topology) RemovePeer(peer PeerID) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.keys, peer)
	delete(t.advertisements, peer)
}

func (t *Topology) Insert(adv SignedAdvertisement, nowMs int64) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if adv.Advertisement.RealmID != t.realm {
		return false, ErrTopologyRealm
	}
	origin := adv.Advertisement.Origin
	key, ok := t.keys[origin]
	if !ok {
		return false, ErrTopologyUntrusted
	}
	if err := adv.Verify(key, nowMs); err != nil {
		return false, err
	}

	if cur, exists := t.advertisements[origin]; exists {
		if cur.Advertisement.Sequence >= adv.Advertisement.Sequence {
			return false, nil
		}
	}
	t.advertisements[origin] = adv
	return true, nil
}

func (t *Topology) Route(destination PeerID, nowMs int64) (Route, bool) {
	routes := t.Routes(nowMs)
	r, ok := routes[destination]
	return r, ok
}

func (t *Topology) Routes(nowMs int64) map[PeerID]Route {
	t.mu.RLock()
	defer t.mu.RUnlock()

	graph := make(map[PeerID][]Neighbor)
	for _, entry := range t.advertisements {
		if entry.Advertisement.ExpiresAt <= nowMs {
			continue
		}
		var validNeighbors []Neighbor
		for _, n := range entry.Advertisement.Neighbors {
			if n.PeerID == t.local || t.keys[n.PeerID] != "" {
				validNeighbors = append(validNeighbors, n)
			}
		}
		graph[entry.Advertisement.Origin] = validNeighbors
	}

	return shortestPaths(graph, t.local)
}

type dijkstraItem struct {
	cost uint64
	peer PeerID
}
type dijkstraHeap []dijkstraItem

func (h dijkstraHeap) Len() int           { return len(h) }
func (h dijkstraHeap) Less(i, j int) bool { return h[i].cost < h[j].cost }
func (h dijkstraHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *dijkstraHeap) Push(x any)        { *h = append(*h, x.(dijkstraItem)) }
func (h *dijkstraHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func shortestPaths(graph map[PeerID][]Neighbor, source PeerID) map[PeerID]Route {
	dist := make(map[PeerID]uint64)
	prev := make(map[PeerID]PeerID)
	pq := &dijkstraHeap{{cost: 0, peer: source}}
	heap.Init(pq)
	dist[source] = 0

	for pq.Len() > 0 {
		top := heap.Pop(pq).(dijkstraItem)
		if cur, ok := dist[top.peer]; ok && cur != top.cost {
			continue
		}
		for _, edge := range graph[top.peer] {
			nextCost := top.cost + uint64(edge.Cost)
			if d, ok := dist[edge.PeerID]; !ok || nextCost < d {
				dist[edge.PeerID] = nextCost
				prev[edge.PeerID] = top.peer
				heap.Push(pq, dijkstraItem{cost: nextCost, peer: edge.PeerID})
			}
		}
	}

	routes := make(map[PeerID]Route)
	for dest, totalCost := range dist {
		if dest == source {
			continue
		}
		var path []PeerID
		curr := dest
		for len(path) <= len(dist) {
			path = append(path, curr)
			p, ok := prev[curr]
			if !ok {
				break
			}
			if p == source {
				path = append(path, source)
				break
			}
			curr = p
		}
		// Reverse
		for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
			path[i], path[j] = path[j], path[i]
		}
		if len(path) >= 2 && path[0] == source {
			routes[dest] = Route{
				Destination: dest,
				NextHop:     path[1],
				TotalCost:   totalCost,
				Path:        path,
			}
		}
	}
	return routes
}
