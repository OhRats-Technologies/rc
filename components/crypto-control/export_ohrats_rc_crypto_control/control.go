package export_ohrats_rc_crypto_control

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"wit_component/ohrats_rc_keys_host_custody"
	"wit_component/ohrats_rc_keys_types"
)

const (
	maxPublicKeyText = 128
	maxContextText   = 256
)

func staticSlot(deviceId string) string {
	return "control:" + deviceId + ":transport"
}

func ensureStaticKey(deviceId string) (*ohrats_rc_keys_host_custody.SecretKey, error) {
	key := ohrats_rc_keys_host_custody.Ensure(staticSlot(deviceId), ohrats_rc_keys_types.KeyAlgorithm_X25519)
	if key.IsErr() {
		return nil, errors.New(key.Err())
	}
	return key.Ok(), nil
}

func publicText(key *ohrats_rc_keys_host_custody.SecretKey) (string, error) {
	publicKey := key.PublicKey()
	if publicKey.IsErr() {
		return "", errors.New(publicKey.Err())
	}
	return encodePublic(publicKey.Ok())
}

func encodePublic(value ohrats_rc_keys_types.PublicKey) (string, error) {
	if value.Algorithm != ohrats_rc_keys_types.KeyAlgorithm_X25519 || len(value.Bytes) != 32 {
		return "", errors.New("control custody returned an invalid X25519 key")
	}
	return base64.RawURLEncoding.EncodeToString(value.Bytes), nil
}

func decodePublic(value string) ([]byte, error) {
	if len(value) > maxPublicKeyText {
		return nil, errors.New("invalid X25519 public key")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return nil, errors.New("invalid X25519 public key")
	}
	return decoded, nil
}

func validateText(value string, label string) error {
	if value == "" || len(value) > maxContextText || strings.Contains(value, "\n") {
		return fmt.Errorf("invalid %s", label)
	}
	return nil
}

func frameNonce(direction uint8, sequence uint64) []byte {
	nonce := make([]byte, 12)
	nonce[0] = direction
	binary.BigEndian.PutUint64(nonce[4:], sequence)
	return nonce
}

func frameAAD(sessionId string, sequence uint64, label string) []byte {
	return []byte(fmt.Sprintf("rc-frame-v1\n%s\n%d\n%s", sessionId, sequence, label))
}

func agree(key *ohrats_rc_keys_host_custody.SecretKey, peerPublic []byte) (*ohrats_rc_keys_host_custody.SharedSecret, error) {
	shared := key.Agree(peerPublic)
	if shared.IsErr() {
		return nil, errors.New(shared.Err())
	}
	return shared.Ok(), nil
}

func openNode(deviceId string, clientId string, challenge string, clientPublicKey string) (NodeOpen, error) {
	if err := validateText(deviceId, "device id"); err != nil {
		return NodeOpen{}, err
	}
	if err := validateText(clientId, "client id"); err != nil {
		return NodeOpen{}, err
	}
	if err := validateText(challenge, "challenge"); err != nil {
		return NodeOpen{}, err
	}
	peer, err := decodePublic(clientPublicKey)
	if err != nil {
		return NodeOpen{}, err
	}
	staticKey, err := ensureStaticKey(deviceId)
	if err != nil {
		return NodeOpen{}, err
	}
	defer staticKey.Drop()
	ephemeral := ohrats_rc_keys_host_custody.Generate(ohrats_rc_keys_types.KeyAlgorithm_X25519)
	if ephemeral.IsErr() {
		return NodeOpen{}, errors.New(ephemeral.Err())
	}
	ephemeralKey := ephemeral.Ok()
	defer ephemeralKey.Drop()
	staticPublic, err := publicText(staticKey)
	if err != nil {
		return NodeOpen{}, err
	}
	ephemeralPublic, err := publicText(ephemeralKey)
	if err != nil {
		return NodeOpen{}, err
	}
	sharedStatic, err := agree(staticKey, peer)
	if err != nil {
		return NodeOpen{}, err
	}
	defer sharedStatic.Drop()
	sharedEphemeral, err := agree(ephemeralKey, peer)
	if err != nil {
		return NodeOpen{}, err
	}
	defer sharedEphemeral.Drop()
	salt := sha256.Sum256([]byte(challenge))
	info := []byte(fmt.Sprintf("rc-e2e-v2\n%s\n%s", deviceId, clientId))
	derived := ohrats_rc_keys_host_custody.Derive(sharedStatic, sharedEphemeral, salt[:], info)
	if derived.IsErr() {
		return NodeOpen{}, errors.New(derived.Err())
	}
	return NodeOpen{
		Session:            &Session{key: derived.Ok()},
		StaticPublicKey:    staticPublic,
		EphemeralPublicKey: ephemeralPublic,
	}, nil
}
