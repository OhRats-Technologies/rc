//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import "sort"

func domainMeshRoutes(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 5 {
		setRejection(results, "invalid-advertisement")
		return
	}
	routes, ok := meshPlanFromArgs(args)
	if !ok {
		setRejection(results, "invalid-advertisement")
		return
	}
	items := make([]*C.wasmtime_component_val_t, 0, len(routes))
	for _, route := range routes {
		items = append(items, meshRouteRecord(route))
	}
	setOK(results, newListValue(items))
}

func domainMeshService(args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t) {
	if nargs < 6 {
		setRejection(results, "invalid-advertisement")
		return
	}
	routes, ok := meshPlanFromArgs(args)
	if !ok {
		setRejection(results, "invalid-advertisement")
		return
	}
	service := valueString(argValue(args, 4))
	byDestination := make(map[string]meshRoute, len(routes))
	for _, route := range routes {
		byDestination[route.destination] = route
	}
	var selected *meshRoute
	var selectedCost uint64
	for _, ad := range meshAdvertisementsFromArgs(args) {
		cost, ok := ad.services[service]
		if !ok {
			continue
		}
		route, ok := byDestination[ad.origin]
		if !ok {
			continue
		}
		total := route.cost + cost
		candidate := route
		candidate.destination = ad.origin
		if selected == nil || total < selectedCost {
			selected, selectedCost = &candidate, total
		}
	}
	option := newValue()
	if selected == nil {
		C.rc_set_none_option(option)
	} else {
		route := meshRouteRecord(*selected)
		value := newOrderedRecord([]fieldValue{
			{name: "service", value: newStringValue(service)},
			{name: "provider-peer-id", value: newStringValue(selected.destination)},
			{name: "route", value: route},
			{name: "total-cost", value: newU64Value(selectedCost)},
		})
		C.rc_set_some_option(option, value)
	}
	setOK(results, option)
}

func meshPlanFromArgs(args *C.wasmtime_component_val_t) ([]meshRoute, bool) {
	advertisements := meshAdvertisementsFromArgs(args)
	if advertisements == nil {
		return nil, false
	}
	realm := valueString(argValue(args, 0))
	local := valueString(argValue(args, 1))
	trusted := meshFeatureSet(parseStrings(argValue(args, 2)))
	now := uint64(C.rc_u64(argValue(args, 4)))
	for _, ad := range advertisements {
		if ad.sequence == 0 || ad.origin == "" || (ad.origin != local && !trusted[ad.origin]) {
			return nil, false
		}
		if !meshIdentifier(realm) || !meshIdentifier(local) {
			return nil, false
		}
		_ = now
	}
	graph := make(map[string]map[string]uint64)
	for _, ad := range advertisements {
		if graph[ad.origin] == nil {
			graph[ad.origin] = make(map[string]uint64)
		}
		for peer, cost := range ad.neighbors {
			graph[ad.origin][peer] = cost
		}
	}
	dist := map[string]uint64{local: 0}
	next := map[string]string{}
	visited := map[string]bool{}
	for {
		current := ""
		var currentCost uint64
		for peer, cost := range dist {
			if !visited[peer] && (current == "" || cost < currentCost) {
				current, currentCost = peer, cost
			}
		}
		if current == "" {
			break
		}
		visited[current] = true
		for peer, cost := range graph[current] {
			nextCost := currentCost + cost
			existing, seen := dist[peer]
			if !seen || nextCost < existing {
				dist[peer] = nextCost
				if current == local {
					next[peer] = peer
				} else {
					next[peer] = next[current]
				}
			}
		}
	}
	routes := make([]meshRoute, 0, len(dist))
	for peer, cost := range dist {
		if peer == local {
			continue
		}
		path := []string{next[peer]}
		for path[len(path)-1] != peer && len(path) <= len(dist) {
			path = append(path, next[path[len(path)-1]])
		}
		routes = append(routes, meshRoute{destination: peer, nextHop: next[peer], cost: cost, path: path})
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].destination < routes[j].destination })
	return routes, true
}

func meshAdvertisementsFromArgs(args *C.wasmtime_component_val_t) []meshAdvertisement {
	realm := valueString(argValue(args, 0))
	local := valueString(argValue(args, 1))
	trusted := meshFeatureSet(parseStrings(argValue(args, 2)))
	now := uint64(C.rc_u64(argValue(args, 4)))
	ads := argValue(args, 3)
	byOrigin := make(map[string]meshAdvertisement)
	for index := 0; index < int(C.rc_list_len(ads)); index++ {
		ad := C.rc_list_get(ads, C.size_t(index))
		origin := fieldString(ad, "origin-peer-id")
		if fieldU32(ad, "version") != 1 || fieldString(ad, "realm-id") != realm ||
			!meshIdentifier(origin) || (origin != local && !trusted[origin]) ||
			fieldU64(ad, "expires-at-ms") <= now {
			continue
		}
		current := byOrigin[origin]
		sequence := fieldU64(ad, "sequence")
		if sequence == 0 || (current.origin != "" && current.sequence >= sequence) {
			continue
		}
		current = meshAdvertisement{origin: origin, sequence: sequence,
			neighbors: make(map[string]uint64), services: make(map[string]uint64)}
		neighbors := fieldList(ad, "neighbors")
		for i := 0; i < int(C.rc_list_len(neighbors)); i++ {
			neighbor := C.rc_list_get(neighbors, C.size_t(i))
			peer := fieldString(neighbor, "peer-id")
			if !meshIdentifier(peer) || fieldU32(neighbor, "cost") == 0 {
				continue
			}
			current.neighbors[peer] = uint64(fieldU32(neighbor, "cost"))
		}
		services := fieldList(ad, "services")
		for i := 0; i < int(C.rc_list_len(services)); i++ {
			service := C.rc_list_get(services, C.size_t(i))
			current.services[fieldString(service, "name")] = uint64(fieldU32(service, "cost"))
		}
		byOrigin[origin] = current
	}
	values := make([]meshAdvertisement, 0, len(byOrigin))
	for _, ad := range byOrigin {
		values = append(values, ad)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].origin < values[j].origin })
	return values
}

func meshRouteRecord(route meshRoute) *C.wasmtime_component_val_t {
	path := make([]*C.wasmtime_component_val_t, len(route.path))
	for index, value := range route.path {
		path[index] = newStringValue(value)
	}
	return newOrderedRecord([]fieldValue{
		{name: "destination-peer-id", value: newStringValue(route.destination)},
		{name: "next-hop-peer-id", value: newStringValue(route.nextHop)},
		{name: "total-cost", value: newU64Value(route.cost)},
		{name: "path", value: newListValue(path)},
	})
}
