package apiclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/OhRats-Technologies/rc/pkg/crypto"
)

var b64URL = base64.RawURLEncoding

var (
	ErrMissingCredential = errors.New("RC credential required; sign in with rc login, or pass a PoP API key with --token / RC_API_TOKEN")
	ErrInvalidPop        = errors.New("invalid RC API signing key")
)

type CredentialKind int

const (
	CredentialBearer CredentialKind = iota
	CredentialPop
)

type Credential struct {
	Kind   CredentialKind
	Bearer string
	Pop    *PopKey
}

type PopKey struct {
	ID      string
	Signing ed25519.PrivateKey
}

func ParseCredential(value string) (Credential, error) {
	val := strings.TrimSpace(value)
	if val == "" {
		return Credential{}, ErrMissingCredential
	}
	if strings.HasPrefix(val, "rcsk_") {
		pop, err := ParsePopKey(val)
		if err != nil {
			return Credential{}, err
		}
		return Credential{Kind: CredentialPop, Pop: pop}, nil
	}
	return Credential{Kind: CredentialBearer, Bearer: val}, nil
}

func CredentialFromSigningSeed(id, seed string) (Credential, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Credential{}, ErrInvalidPop
	}
	seedBytes, err := b64URL.DecodeString(strings.TrimSpace(seed))
	if err != nil || len(seedBytes) != 32 {
		return Credential{}, ErrInvalidPop
	}
	key := ed25519.NewKeyFromSeed(seedBytes)
	return Credential{
		Kind: CredentialPop,
		Pop: &PopKey{
			ID:      id,
			Signing: key,
		},
	}, nil
}

func ParsePopKey(secret string) (*PopKey, error) {
	value := strings.TrimPrefix(secret, "rcsk_")
	parts := strings.SplitN(value, "_", 2)
	if len(parts) != 2 || parts[0] == "" {
		return nil, ErrInvalidPop
	}
	der, err := b64URL.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidPop
	}

	pk, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, ErrInvalidPop
	}
	edKey, ok := pk.(ed25519.PrivateKey)
	if !ok {
		return nil, ErrInvalidPop
	}
	return &PopKey{
		ID:      parts[0],
		Signing: edKey,
	}, nil
}

func (k *PopKey) Headers(method, requestURI string, body []byte) map[string]string {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := RandomURLBytes(18)
	payload := crypto.ApiPayload(k.ID, timestamp, nonce, method, requestURI, body)
	sig := ed25519.Sign(k.Signing, []byte(payload))
	signature := b64URL.EncodeToString(sig)

	return map[string]string{
		"x-rc-key-id":    k.ID,
		"x-rc-timestamp": timestamp,
		"x-rc-nonce":     nonce,
		"x-rc-signature": signature,
	}
}

func RandomURLBytes(size int) string {
	b := make([]byte, size)
	_, _ = rand.Read(b)
	return b64URL.EncodeToString(b)
}

func (k *PopKey) PublicKeyString() string {
	pub := k.Signing.Public().(ed25519.PublicKey)
	return b64URL.EncodeToString(pub)
}
