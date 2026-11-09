package server

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/protocol"
)

type pendingNodeImage struct {
	deviceID string
	bytes    []byte
	channel  chan nodeImageResult
}

type nodeImage struct {
	mimeType string
	data     []byte
}

type nodeImageResult struct {
	image nodeImage
	err   error
}

func (h *NodeHub) Image(ctx context.Context, deviceID, path, grantJSON, signature, controlGrant, credentialID, assertion string) (nodeImage, error) {
	requestID := randomID(18)
	channel := make(chan nodeImageResult, 1)
	h.mu.Lock()
	h.images[requestID] = &pendingNodeImage{deviceID: deviceID, channel: channel}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.images, requestID)
		h.mu.Unlock()
	}()
	if err := h.send(deviceID, protocol.ServerToNode{
		Type: "mcp_image", RequestID: requestID, Path: path,
		McpGrant: grantJSON, McpSignature: signature,
		ControlGrant: controlGrant, CredentialID: credentialID, ControlAssertion: assertion,
	}); err != nil {
		return nodeImage{}, err
	}
	select {
	case result := <-channel:
		return result.image, result.err
	case <-ctx.Done():
		return nodeImage{}, ctx.Err()
	case <-time.After(60 * time.Second):
		return nodeImage{}, errors.New("Node image request timed out")
	}
}

func (h *NodeHub) dispatch(deviceID string, message protocol.NodeToServer) {
	switch message.Type {
	case "hello":
		if message.Hello != nil && h.onHello != nil {
			h.onHello(deviceID, message.Hello)
		}
	case "lock_state":
		if h.onLock != nil {
			h.onLock(deviceID, message.Hash, message.Generation)
		}
	case protocol.TypeControlClosed:
		if h.onControlClosed != nil {
			h.onControlClosed(deviceID, message.SessionID)
		}
	case protocol.TypeProcessState:
		if h.onProcessState != nil {
			h.onProcessState(deviceID, message)
		}
	}
	if message.Type == "mcp_image_chunk" || message.Type == "mcp_image_result" {
		h.dispatchImage(deviceID, message)
		return
	}
	if message.RequestID == "" {
		return
	}
	h.mu.Lock()
	pending, ok := h.pending[message.RequestID]
	h.mu.Unlock()
	if !ok || pending.deviceID != deviceID {
		return
	}
	select {
	case pending.channel <- nodeResponse{message: message}:
	default:
	}
}

func (h *NodeHub) dispatchImage(deviceID string, message protocol.NodeToServer) {
	h.mu.Lock()
	pending := h.images[message.RequestID]
	if pending == nil || pending.deviceID != deviceID {
		h.mu.Unlock()
		return
	}
	switch message.Type {
	case "mcp_image_chunk":
		data, err := base64.RawURLEncoding.DecodeString(message.Data)
		if err != nil {
			delete(h.images, message.RequestID)
			h.mu.Unlock()
			select {
			case pending.channel <- nodeImageResult{err: errors.New("invalid image chunk")}:
			default:
			}
			return
		}
		pending.bytes = append(pending.bytes, data...)
		if len(pending.bytes) > protocol.McpImageMaxBytes {
			delete(h.images, message.RequestID)
			h.mu.Unlock()
			select {
			case pending.channel <- nodeImageResult{err: errors.New("image exceeds the configured limit")}:
			default:
			}
			return
		}
		h.mu.Unlock()
	case "mcp_image_result":
		if message.Error != "" {
			delete(h.images, message.RequestID)
			h.mu.Unlock()
			select {
			case pending.channel <- nodeImageResult{err: errors.New(message.Error)}:
			default:
			}
			return
		}
		if uint64(len(pending.bytes)) != message.SizeBytes {
			delete(h.images, message.RequestID)
			h.mu.Unlock()
			select {
			case pending.channel <- nodeImageResult{err: errors.New("incomplete image transfer")}:
			default:
			}
			return
		}
		image := nodeImage{mimeType: message.MimeType, data: append([]byte(nil), pending.bytes...)}
		delete(h.images, message.RequestID)
		h.mu.Unlock()
		select {
		case pending.channel <- nodeImageResult{image: image}:
		default:
		}
	default:
		h.mu.Unlock()
	}
}

func (h *NodeHub) Close() {
	h.mu.Lock()
	peers := make([]*nodePeer, 0, len(h.peers))
	deviceIDs := make([]string, 0, len(h.peers))
	for deviceID, peer := range h.peers {
		peers = append(peers, peer)
		deviceIDs = append(deviceIDs, deviceID)
	}
	h.peers = make(map[string]*nodePeer)
	h.mu.Unlock()
	for _, peer := range peers {
		_ = peer.conn.Close()
	}
	for _, deviceID := range deviceIDs {
		h.failDevice(deviceID)
	}
}
