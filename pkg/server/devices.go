package server

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	authstore "github.com/OhRats-Technologies/rc/pkg/auth"
	"github.com/OhRats-Technologies/rc/pkg/node"
)

type DeviceRegistry struct {
	mu      sync.RWMutex
	devices map[string]*RegisteredDevice
	tokens  map[string]bool // one-time development enrollment tokens
	nonces  map[string]time.Time
	// Consume resolves a durable enrollment token to its workspace.
	Consume func(string) (string, error)
	// store persists devices; nil keeps the registry in memory only.
	store *authstore.Store
}

type RegisteredDevice struct {
	ID                 string    `json:"id"`
	WorkspaceID        string    `json:"workspaceId"`
	Name               string    `json:"name"`
	Hostname           string    `json:"hostname"`
	Platform           string    `json:"platform"`
	Arch               string    `json:"arch"`
	Version            string    `json:"version"`
	Capabilities       []string  `json:"capabilities"`
	IdentityPublicKey  string    `json:"identityPublicKey"`
	TransportPublicKey string    `json:"transportPublicKey"`
	Online             bool      `json:"online"`
	LastSeen           time.Time `json:"lastSeen"`
}

func NewDeviceRegistry() *DeviceRegistry {
	return &DeviceRegistry{
		devices: make(map[string]*RegisteredDevice),
		tokens:  make(map[string]bool),
		nonces:  make(map[string]time.Time),
	}
}

// AddToken registers a one-time enrollment token. Tokens only exist in the
// development registry; production enrollment tokens are stored as hashes by the
// RC server implementation.
func (r *DeviceRegistry) AddToken(token string) {
	if token == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[token] = true
}

func (r *DeviceRegistry) GenerateToken() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	r.tokens[tok] = true
	return tok
}

func (r *DeviceRegistry) HandleEnroll(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var enrollReq node.EnrollRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 64<<10)).Decode(&enrollReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := validateEnrollRequest(&enrollReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	workspaceID := defaultWorkspaceID
	r.mu.Lock()
	if r.tokens[enrollReq.Token] {
		delete(r.tokens, enrollReq.Token)
		r.mu.Unlock()
	} else {
		consume := r.Consume
		r.mu.Unlock()
		if consume == nil {
			http.Error(w, "invalid or expired enrollment token", http.StatusForbidden)
			return
		}
		resolved, err := consume(enrollReq.Token)
		if err != nil || resolved == "" {
			http.Error(w, "invalid or expired enrollment token", http.StatusForbidden)
			return
		}
		workspaceID = resolved
	}

	randBytes := make([]byte, 8)
	_, _ = rand.Read(randBytes)
	deviceID := fmt.Sprintf("dev_%s", hex.EncodeToString(randBytes))

	dev := &RegisteredDevice{
		ID:                 deviceID,
		WorkspaceID:        workspaceID,
		Name:               enrollReq.Name,
		Hostname:           enrollReq.Hostname,
		Platform:           enrollReq.Platform,
		Arch:               enrollReq.Arch,
		Version:            enrollReq.Version,
		Capabilities:       enrollReq.Capabilities,
		IdentityPublicKey:  enrollReq.IdentityPublicKey,
		TransportPublicKey: enrollReq.TransportPublicKey,
		Online:             true,
		LastSeen:           time.Now(),
	}
	if err := r.persistEnrollment(dev); err != nil {
		http.Error(w, "device could not be stored", http.StatusInternalServerError)
		return
	}
	r.mu.Lock()
	r.devices[deviceID] = dev
	r.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"deviceId": deviceID,
	})
}

func (r *DeviceRegistry) HandleListDevices(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"devices": r.List()})
}

// List returns registered devices ordered by workspace and name.
func (r *DeviceRegistry) List() []*RegisteredDevice {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]*RegisteredDevice, 0, len(r.devices))
	for _, device := range r.devices {
		copied := *device
		list = append(list, &copied)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].WorkspaceID != list[j].WorkspaceID {
			return list[i].WorkspaceID < list[j].WorkspaceID
		}
		return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
	})
	return list
}

// Get returns one registered device.
func (r *DeviceRegistry) Get(id string) (*RegisteredDevice, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	device, ok := r.devices[id]
	if !ok {
		return nil, false
	}
	copied := *device
	return &copied, true
}

// Rename changes a device display name.
func (r *DeviceRegistry) Rename(id, name string) (*RegisteredDevice, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 120 {
		return nil, errors.New("invalid device name")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	device, ok := r.devices[id]
	if !ok {
		return nil, errors.New("device not found")
	}
	if r.store != nil {
		if err := r.store.RenameDevice(id, name); err != nil {
			return nil, err
		}
	}
	device.Name = name
	copied := *device
	return &copied, nil
}

func (r *DeviceRegistry) HandleDeviceRoutes(w http.ResponseWriter, req *http.Request) {
	id := strings.TrimPrefix(req.URL.Path, "/api/v1/devices/")
	if id == "" {
		r.HandleListDevices(w, req)
		return
	}

	dev, ok := r.Get(id)
	if !ok {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}

	switch req.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"device": dev})
	case http.MethodPatch:
		var input struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, req.Body, 8<<10)).Decode(&input); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		renamed, err := r.Rename(id, input.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": renamed.Name})
	case http.MethodDelete:
		if err := r.remove(id); err != nil {
			if errors.Is(err, authstore.ErrDeviceNotFound) {
				http.Error(w, "device not found", http.StatusNotFound)
				return
			}
			http.Error(w, "device could not be removed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func validateEnrollRequest(req *node.EnrollRequest) error {
	if strings.TrimSpace(req.Token) == "" {
		return errors.New("enrollment token required")
	}
	if len(req.Name) > 128 {
		return errors.New("device name exceeds 128 bytes")
	}
	identity, err := base64.RawURLEncoding.DecodeString(req.IdentityPublicKey)
	if err != nil || len(identity) != ed25519.PublicKeySize {
		return errors.New("identityPublicKey must be a base64url Ed25519 public key")
	}
	transport, err := base64.RawURLEncoding.DecodeString(req.TransportPublicKey)
	if err != nil || len(transport) != 32 {
		return errors.New("transportPublicKey must be a base64url X25519 public key")
	}
	return nil
}
