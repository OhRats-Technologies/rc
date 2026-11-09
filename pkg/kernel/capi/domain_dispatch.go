//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"errors"
	"sync"
)

var errDomainConflict = errors.New("domain state changed concurrently")

var (
	domainActive     *domainState
	domainDispatchMu sync.Mutex
)

func domainStore() *domainState { return domainActive }

func domainDispatch(id uint64, name string, args *C.wasmtime_component_val_t, nargs C.size_t, results *C.wasmtime_component_val_t, nresults C.size_t) {
	if nargs < 0 || results == nil || nresults < 1 {
		return
	}
	state := lookupDomainID(id)
	if state == nil {
		setStringResult(results, false, "domain host is unavailable")
		return
	}
	domainDispatchMu.Lock()
	defer domainDispatchMu.Unlock()
	state.mu.Lock()
	domainActive = state
	defer func() {
		domainActive = nil
		state.mu.Unlock()
	}()
	switch name {
	case "mesh.negotiate":
		domainMeshNegotiate(args, nargs, results)
	case "mesh.routes":
		domainMeshRoutes(args, nargs, results)
	case "mesh.service":
		domainMeshService(args, nargs, results)
	case "mesh.validate":
		domainMeshValidate(args, nargs, results)
	case "mesh.forward":
		domainMeshForward(args, nargs, results)
	case "key.ensure":
		domainKeyEnsure(args, nargs, results)
	case "key.lookup":
		domainKeyLookup(args, nargs, results)
	case "key.sign":
		domainKeySign(args, nargs, results)
	case "key.remove":
		domainKeyRemove(args, nargs, results)
	case "crypto.verify":
		domainCryptoVerify(args, nargs, results)
	case "crypto.sha256":
		domainCryptoSHA256(args, nargs, results)
	case "identity.user.create":
		domainIdentityUserCreate(args, nargs, results)
	case "identity.user.get":
		domainIdentityUserGet(args, nargs, results)
	case "identity.user.all":
		domainIdentityUserAll(args, nargs, results)
	case "identity.user.count":
		domainIdentityUserCount(args, nargs, results)
	case "identity.user.rename":
		domainIdentityUserRename(args, nargs, results)
	case "identity.credential.create-user":
		domainIdentityCredentialCreateUser(args, nargs, results)
	case "identity.credential.add":
		domainIdentityCredentialAdd(args, nargs, results)
	case "identity.credential.get":
		domainIdentityCredentialGet(args, nargs, results)
	case "identity.credential.all":
		domainIdentityCredentialAll(args, nargs, results)
	case "identity.credential.update":
		domainIdentityCredentialUpdate(args, nargs, results)
	case "identity.credential.remove":
		domainIdentityCredentialRemove(args, nargs, results)
	case "identity.ceremony.put":
		domainIdentityCeremonyPut(args, nargs, results)
	case "identity.ceremony.take":
		domainIdentityCeremonyTake(args, nargs, results)
	case "identity.admin.begin":
		domainIdentityAdminBegin(args, nargs, results)
	case "identity.admin.issue":
		domainIdentityAdminIssue(args, nargs, results)
	case "session.find":
		domainSessionFind(args, nargs, results)
	case "session.issue":
		domainSessionIssue(args, nargs, results)
	case "session.revoke":
		domainSessionRevoke(args, nargs, results)
	case "api.create":
		domainAPICreate(args, nargs, results)
	case "api.all":
		domainAPIAll(args, nargs, results)
	case "api.get":
		domainAPIGet(args, nargs, results)
	case "api.revoke":
		domainAPIRevoke(args, nargs, results)
	case "api.verify":
		domainAPIVerify(args, nargs, results)
	case "api.start-cli":
		domainAPIStartCLI(args, nargs, results)
	case "api.approve-cli":
		domainAPIApproveCLI(args, nargs, results)
	case "api.poll-cli":
		domainAPIPollCLI(args, nargs, results)
	case "events.append":
		domainEventsAppend(args, nargs, results)
	case "events.query":
		domainEventsQuery(args, nargs, results)
	case "events.feed":
		domainEventsQuery(args, nargs, results)
	case "events.retention.configure":
		domainEventsConfigure(args, nargs, results)
	case "events.retention.current":
		domainEventsCurrent(args, nargs, results)
	case "component.installed":
		domainComponentInstalled(args, nargs, results)
	case "component.install", "component.prepare", "component.commit", "component.abort",
		"component.remove", "cache.read", "cache.write", "files.read", "catalog.read",
		"services.providers", "source.resolve", "artifact.fetch", "state.read",
		"state.write", "state.remove":
		setStringResult(results, false, "package installation is out of scope for the control-authority host")
	default:
		setStringResult(results, false, "domain host is unavailable")
	}
}
