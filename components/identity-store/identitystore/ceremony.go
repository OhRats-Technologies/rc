package identitystore

import (
	"errors"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_identity_types"
)

const ceremonyBucket = "ceremonies"

type storedCeremony struct {
	Id          string    `json:"id"`
	Kind        string    `json:"kind"`
	UserId      *string   `json:"user_id"`
	Metadata    byteArray `json:"metadata"`
	State       byteArray `json:"state"`
	ExpiresAtMs uint64    `json:"expires_at_ms"`
}

func (value storedCeremony) view() ohrats_rc_identity_types.Ceremony {
	ceremony := ohrats_rc_identity_types.Ceremony{
		Id:          value.Id,
		Kind:        value.Kind,
		UserId:      witTypes.None[string](),
		Metadata:    value.Metadata,
		State:       value.State,
		ExpiresAtMs: value.ExpiresAtMs,
	}
	if value.UserId != nil {
		ceremony.UserId = witTypes.Some(*value.UserId)
	}
	return ceremony
}

func PutCeremony(value ohrats_rc_identity_types.Ceremony) error {
	if err := validID(value.Id, "ceremony id"); err != nil {
		return err
	}
	if err := validKind(value.Kind); err != nil {
		return err
	}
	var userId *string
	if value.UserId.IsSome() {
		id := value.UserId.Some()
		if err := validID(id, "user id"); err != nil {
			return err
		}
		userId = &id
	}
	if err := validCeremonyPayload(value.Metadata, value.State); err != nil {
		return err
	}
	if value.ExpiresAtMs <= nowMs() {
		return errors.New("ceremony expiration must be in the future")
	}
	stored := storedCeremony{
		Id:          value.Id,
		Kind:        value.Kind,
		UserId:      userId,
		Metadata:    emptyByteArray(value.Metadata),
		State:       emptyByteArray(value.State),
		ExpiresAtMs: value.ExpiresAtMs,
	}
	encoded, err := encode(stored)
	if err != nil {
		return err
	}
	return insert(ceremonyBucket, ceremonyKey(value.Id, value.Kind), encoded)
}

func TakeCeremony(id string, kind string) (witTypes.Option[ohrats_rc_identity_types.Ceremony], error) {
	if err := validID(id, "ceremony id"); err != nil {
		return witTypes.None[ohrats_rc_identity_types.Ceremony](), err
	}
	if err := validKind(kind); err != nil {
		return witTypes.None[ohrats_rc_identity_types.Ceremony](), err
	}
	value, found, err := takeValue(ceremonyBucket, ceremonyKey(id, kind))
	if err != nil || !found {
		return witTypes.None[ohrats_rc_identity_types.Ceremony](), err
	}
	stored, err := decode[storedCeremony](value)
	if err != nil {
		return witTypes.None[ohrats_rc_identity_types.Ceremony](), err
	}
	if stored.ExpiresAtMs <= nowMs() {
		return witTypes.None[ohrats_rc_identity_types.Ceremony](), nil
	}
	return witTypes.Some(stored.view()), nil
}

func ceremonyKey(id string, kind string) []byte {
	key := make([]byte, 0, len(kind)+len(id)+1)
	key = append(key, kind...)
	key = append(key, 0)
	key = append(key, id...)
	return key
}
