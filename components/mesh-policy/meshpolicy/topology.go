package meshpolicy

import (
	"container/heap"
	"sort"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_mesh_types"
)

type edge struct {
	peer string
	cost uint32
}

type queueItem struct {
	cost uint64
	peer string
}

type queue []queueItem

func (values queue) Len() int { return len(values) }

func (values queue) Less(left int, right int) bool {
	if values[left].cost != values[right].cost {
		return values[left].cost < values[right].cost
	}
	return values[left].peer < values[right].peer
}

func (values queue) Swap(left int, right int) {
	values[left], values[right] = values[right], values[left]
}

func (values *queue) Push(value any) {
	*values = append(*values, value.(queueItem))
}

func (values *queue) Pop() any {
	previous := *values
	last := len(previous) - 1
	value := previous[last]
	*values = previous[:last]
	return value
}

// Routes computes the lowest-cost trusted path from the local peer to every
// reachable peer.
func Routes(realmId string, localPeerId string, trustedPeerIds []string, advertisements []ohrats_rc_mesh_types.LinkAdvertisement, nowMs uint64) witTypes.Result[[]ohrats_rc_mesh_types.Route, ohrats_rc_mesh_types.Rejection] {
	routes, err := routes(realmId, localPeerId, trustedPeerIds, advertisements, nowMs)
	if err != nil {
		return witTypes.Err[[]ohrats_rc_mesh_types.Route, ohrats_rc_mesh_types.Rejection](rejectionOf(err))
	}
	return witTypes.Ok[[]ohrats_rc_mesh_types.Route, ohrats_rc_mesh_types.Rejection](routes)
}

func routes(realmId string, localPeerId string, trustedPeerIds []string, advertisements []ohrats_rc_mesh_types.LinkAdvertisement, nowMs uint64) ([]ohrats_rc_mesh_types.Route, error) {
	if err := identifier(realmId); err != nil {
		return nil, err
	}
	if err := identifier(localPeerId); err != nil {
		return nil, err
	}
	trusted := make(map[string]struct{}, len(trustedPeerIds))
	for _, peer := range trustedPeerIds {
		if err := identifier(peer); err != nil {
			return nil, err
		}
		trusted[peer] = struct{}{}
	}
	origins := make(map[string]struct{}, len(advertisements))
	graph := make(map[string][]edge, len(advertisements))
	for _, item := range advertisements {
		if err := advertisement(item); err != nil {
			return nil, err
		}
		if item.RealmId != realmId {
			return nil, reject(ohrats_rc_mesh_types.MakeRejectionRealmMismatch())
		}
		if item.ExpiresAtMs <= nowMs {
			continue
		}
		if item.OriginPeerId != localPeerId {
			if _, ok := trusted[item.OriginPeerId]; !ok {
				return nil, reject(ohrats_rc_mesh_types.MakeRejectionUntrustedPeer())
			}
		}
		if _, duplicate := origins[item.OriginPeerId]; duplicate {
			return nil, reject(ohrats_rc_mesh_types.MakeRejectionInvalidAdvertisement())
		}
		origins[item.OriginPeerId] = struct{}{}
		links := make([]edge, 0, len(item.Neighbors))
		for _, neighbor := range item.Neighbors {
			if neighbor.PeerId == localPeerId {
				links = append(links, edge{peer: neighbor.PeerId, cost: neighbor.Cost})
				continue
			}
			if _, ok := trusted[neighbor.PeerId]; ok {
				links = append(links, edge{peer: neighbor.PeerId, cost: neighbor.Cost})
			}
		}
		graph[item.OriginPeerId] = links
	}
	return shortestPaths(graph, localPeerId), nil
}

// Service selects the cheapest advertised provider for a service name.
func Service(realmId string, localPeerId string, trustedPeerIds []string, advertisements []ohrats_rc_mesh_types.LinkAdvertisement, service string, nowMs uint64) witTypes.Result[witTypes.Option[ohrats_rc_mesh_types.ServiceRoute], ohrats_rc_mesh_types.Rejection] {
	if len(service) == 0 || len(service) > 128 {
		return witTypes.Err[witTypes.Option[ohrats_rc_mesh_types.ServiceRoute], ohrats_rc_mesh_types.Rejection](ohrats_rc_mesh_types.MakeRejectionInvalidAdvertisement())
	}
	planned, err := routes(realmId, localPeerId, trustedPeerIds, advertisements, nowMs)
	if err != nil {
		return witTypes.Err[witTypes.Option[ohrats_rc_mesh_types.ServiceRoute], ohrats_rc_mesh_types.Rejection](rejectionOf(err))
	}
	byPeer := make(map[string]ohrats_rc_mesh_types.Route, len(planned))
	for _, route := range planned {
		byPeer[route.DestinationPeerId] = route
	}
	var best *ohrats_rc_mesh_types.ServiceRoute
	for _, item := range advertisements {
		if item.ExpiresAtMs <= nowMs {
			continue
		}
		advertised := false
		var cost uint32
		for _, candidate := range item.Services {
			if candidate.Name == service {
				advertised = true
				cost = candidate.Cost
				break
			}
		}
		if !advertised {
			continue
		}
		route, reachable := byPeer[item.OriginPeerId]
		if !reachable {
			continue
		}
		candidate := ohrats_rc_mesh_types.ServiceRoute{
			Service:        service,
			ProviderPeerId: item.OriginPeerId,
			Route:          route,
			TotalCost:      saturatingAdd(route.TotalCost, uint64(cost)),
		}
		if best == nil ||
			candidate.TotalCost < best.TotalCost ||
			(candidate.TotalCost == best.TotalCost && candidate.ProviderPeerId < best.ProviderPeerId) {
			selected := candidate
			best = &selected
		}
	}
	if best == nil {
		return witTypes.Ok[witTypes.Option[ohrats_rc_mesh_types.ServiceRoute], ohrats_rc_mesh_types.Rejection](witTypes.None[ohrats_rc_mesh_types.ServiceRoute]())
	}
	return witTypes.Ok[witTypes.Option[ohrats_rc_mesh_types.ServiceRoute], ohrats_rc_mesh_types.Rejection](witTypes.Some(*best))
}

func shortestPaths(graph map[string][]edge, source string) []ohrats_rc_mesh_types.Route {
	distance := map[string]uint64{source: 0}
	previous := make(map[string]string)
	pending := &queue{{cost: 0, peer: source}}
	heap.Init(pending)
	for pending.Len() > 0 {
		item := heap.Pop(pending).(queueItem)
		current, known := distance[item.peer]
		if !known || current != item.cost {
			continue
		}
		for _, link := range graph[item.peer] {
			next := saturatingAdd(current, uint64(link.cost))
			if existing, seen := distance[link.peer]; !seen || next < existing {
				distance[link.peer] = next
				previous[link.peer] = item.peer
				heap.Push(pending, queueItem{cost: next, peer: link.peer})
			}
		}
	}
	destinations := make([]string, 0, len(distance))
	for destination := range distance {
		if destination != source {
			destinations = append(destinations, destination)
		}
	}
	sort.Strings(destinations)
	planned := make([]ohrats_rc_mesh_types.Route, 0, len(destinations))
	for _, destination := range destinations {
		path := []string{destination}
		cursor := destination
		for {
			parent, found := previous[cursor]
			if !found {
				break
			}
			path = append(path, parent)
			if parent == source {
				break
			}
			cursor = parent
		}
		for left, right := 0, len(path)-1; left < right; left, right = left+1, right-1 {
			path[left], path[right] = path[right], path[left]
		}
		if len(path) >= 2 && path[0] == source {
			planned = append(planned, ohrats_rc_mesh_types.Route{
				DestinationPeerId: destination,
				NextHopPeerId:     path[1],
				TotalCost:         distance[destination],
				Path:              path,
			})
		}
	}
	return planned
}
