//go:build cgo && !windows

package capi

/*
#include "capi.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

var errDomainCallback = errors.New("domain callback is unavailable")

type domainFunc struct {
	instance string
	name     string
	handler  string
}

func linkDomainHosts(root unsafe.Pointer, slot unsafe.Pointer) error {
	id := uint64(*(*C.uint64_t)(slot))
	functions := domainFunctions()
	seen := make(map[string]bool, len(functions))
	for _, function := range functions {
		if seen[function.instance] {
			continue
		}
		seen[function.instance] = true
		handle, err := addInstance(root, function.instance)
		if err != nil {
			return err
		}
		for _, entry := range functions {
			if entry.instance != function.instance {
				continue
			}
			binding := newDomainBinding(id, entry.handler)
			if err := addFuncData(handle, entry.name, C.rc_domain_callback(), binding); err != nil {
				C.wasmtime_component_linker_instance_delete(handle)
				return err
			}
		}
		C.wasmtime_component_linker_instance_delete(handle)
	}
	return nil
}

func domainFunctions() []domainFunc {
	return []domainFunc{
		{"ohrats:rc-mesh/capabilities@0.1.0", "negotiate", "mesh.negotiate"},
		{"ohrats:rc-mesh/topology@0.1.0", "routes", "mesh.routes"},
		{"ohrats:rc-mesh/topology@0.1.0", "service", "mesh.service"},
		{"ohrats:rc-mesh/envelopes@0.1.0", "validate", "mesh.validate"},
		{"ohrats:rc-mesh/envelopes@0.1.0", "forward", "mesh.forward"},
		{"ohrats:rc-keys/custody@0.1.0", "ensure", "key.ensure"},
		{"ohrats:rc-keys/custody@0.1.0", "lookup", "key.lookup"},
		{"ohrats:rc-keys/custody@0.1.0", "sign", "key.sign"},
		{"ohrats:rc-keys/custody@0.1.0", "remove", "key.remove"},
		{"ohrats:rc-crypto/verifier@0.1.0", "verify", "crypto.verify"},
		{"ohrats:rc-crypto/verifier@0.1.0", "sha256", "crypto.sha256"},
		{"ohrats:rc-identity/users@0.1.0", "create", "identity.user.create"},
		{"ohrats:rc-identity/users@0.1.0", "get", "identity.user.get"},
		{"ohrats:rc-identity/users@0.1.0", "all", "identity.user.all"},
		{"ohrats:rc-identity/users@0.1.0", "count", "identity.user.count"},
		{"ohrats:rc-identity/users@0.1.0", "rename", "identity.user.rename"},
		{"ohrats:rc-identity/credentials@0.1.0", "create-user", "identity.credential.create-user"},
		{"ohrats:rc-identity/credentials@0.1.0", "add", "identity.credential.add"},
		{"ohrats:rc-identity/credentials@0.1.0", "get-by-credential-id", "identity.credential.get"},
		{"ohrats:rc-identity/credentials@0.1.0", "all", "identity.credential.all"},
		{"ohrats:rc-identity/credentials@0.1.0", "update", "identity.credential.update"},
		{"ohrats:rc-identity/credentials@0.1.0", "remove", "identity.credential.remove"},
		{"ohrats:rc-identity/ceremonies@0.1.0", "put", "identity.ceremony.put"},
		{"ohrats:rc-identity/ceremonies@0.1.0", "take", "identity.ceremony.take"},
		{"ohrats:rc-identity/admin-issuer@0.1.0", "begin", "identity.admin.begin"},
		{"ohrats:rc-identity/admin-issuer@0.1.0", "issue", "identity.admin.issue"},
		{"ohrats:rc-session/lookup@0.1.0", "find", "session.find"},
		{"ohrats:rc-session/management@0.1.0", "issue", "session.issue"},
		{"ohrats:rc-session/management@0.1.0", "revoke", "session.revoke"},
		{"ohrats:rc-api-credentials/credentials@0.1.0", "create-api", "api.create"},
		{"ohrats:rc-api-credentials/credentials@0.1.0", "all", "api.all"},
		{"ohrats:rc-api-credentials/credentials@0.1.0", "get", "api.get"},
		{"ohrats:rc-api-credentials/credentials@0.1.0", "revoke", "api.revoke"},
		{"ohrats:rc-api-credentials/credentials@0.1.0", "verify", "api.verify"},
		{"ohrats:rc-api-credentials/credentials@0.1.0", "start-cli", "api.start-cli"},
		{"ohrats:rc-api-credentials/credentials@0.1.0", "approve-cli", "api.approve-cli"},
		{"ohrats:rc-api-credentials/credentials@0.1.0", "poll-cli", "api.poll-cli"},
		{"ohrats:rc-api-credentials/credentials@0.1.0", "revoke-cli", "api.revoke-cli"},
		{"ohrats:rc-events/append@0.1.0", "append", "events.append"},
		{"ohrats:rc-events/query@0.1.0", "query", "events.query"},
		{"ohrats:rc-events/feed@0.1.0", "poll", "events.feed"},
		{"ohrats:rc-events/retention@0.1.0", "configure", "events.retention.configure"},
		{"ohrats:rc-events/retention@0.1.0", "current", "events.retention.current"},
		{"ohrats:rc-plugin/component-store@0.1.0", "installed", "component.installed"},
		{"ohrats:rc-plugin/component-store@0.1.0", "install", "component.install"},
		{"ohrats:rc-plugin/component-store@0.1.0", "prepare", "component.prepare"},
		{"ohrats:rc-plugin/component-store@0.1.0", "commit", "component.commit"},
		{"ohrats:rc-plugin/component-store@0.1.0", "abort", "component.abort"},
		{"ohrats:rc-plugin/component-store@0.1.0", "remove", "component.remove"},
		{"ohrats:rc-plugin/artifact-cache@0.1.0", "read", "cache.read"},
		{"ohrats:rc-plugin/artifact-cache@0.1.0", "write", "cache.write"},
		{"ohrats:rc-plugin/state-store@0.1.0", "read", "state.read"},
		{"ohrats:rc-plugin/state-store@0.1.0", "write", "state.write"},
		{"ohrats:rc-plugin/state-store@0.1.0", "remove", "state.remove"},
		{"ohrats:rc-plugin/local-files@0.1.0", "read", "files.read"},
		{"ohrats:rc-plugin/catalog-store@0.1.0", "read", "catalog.read"},
		{"ohrats:rc-plugin/service-registry@0.1.0", "providers", "services.providers"},
		{"ohrats:rc-plugin/package-source@0.1.0", "resolve", "source.resolve"},
		{"ohrats:rc-artifact-cache/cache@0.1.0", "fetch", "artifact.fetch"},
	}
}
