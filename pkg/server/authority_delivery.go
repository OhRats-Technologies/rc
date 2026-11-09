package server

import (
	"encoding/json"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

func (s *Server) handleNodeHello(deviceID string, hello *protocol.NodeHello) {
	if hello == nil {
		return
	}
	_ = s.registry.UpdateHello(deviceID, hello.Version, hello.Capabilities)
	_ = s.registry.UpdateLockState(deviceID, hello.LockHash, hello.LockGeneration)
	s.deliverAuthority(deviceID)
}

func (s *Server) handleNodeLockState(deviceID, hash string, generation uint64) {
	_ = s.registry.UpdateLockState(deviceID, hash, generation)
	_ = s.auth.DeleteAuthorityDeliveries(deviceID, hash, generation)
	s.deliverAuthority(deviceID)
}

func (s *Server) deliverAuthority(deviceID string) bool {
	device, ok := s.registry.Get(deviceID)
	if !ok {
		return false
	}
	if device.LockHash == "" {
		snapshot, err := s.authoritySnapshot(device.WorkspaceID)
		if err != nil {
			return false
		}
		return s.nodes.Send(deviceID, protocol.ServerToNode{Type: "lock_bootstrap", Snapshot: snapshot}) == nil
	}
	delivery, err := s.auth.AuthorityDelivery(deviceID, device.LockHash, device.LockGeneration)
	if err != nil || delivery == nil {
		return false
	}
	var message protocol.ServerToNode
	if json.Unmarshal([]byte(delivery.Message), &message) != nil {
		return false
	}
	return s.nodes.Send(deviceID, message) == nil
}
