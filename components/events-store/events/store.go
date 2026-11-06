package events

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"

	"wit_component/ohrats_rc_events_types"
	storageTypes "wit_component/ohrats_rc_storage_types"
)

const (
	nextCursorKey = "next-cursor"
	policyKey     = "maximum-events"

	// DefaultMax is the retention limit used before an explicit policy exists.
	DefaultMax uint32 = 10_000
	// MaxRetention bounds the configurable retention policy.
	MaxRetention uint32 = 100_000

	pruneBatch = 500
)

// Append validates, deduplicates and durably stores one lifecycle event.
func Append(request ohrats_rc_events_types.AppendRequest) (ohrats_rc_events_types.Event, error) {
	if err := ValidateRequest(request); err != nil {
		return ohrats_rc_events_types.Event{}, err
	}
	var idempotency []byte
	if request.IdempotencyKey.IsSome() {
		idempotency = idempotencyKey(request.IdempotencyKey.Some())
	}
	fingerprint, err := requestFingerprint(request)
	if err != nil {
		return ohrats_rc_events_types.Event{}, err
	}
	if idempotency != nil {
		value, present, err := get(IdempotencyBucket, idempotency)
		if err != nil {
			return ohrats_rc_events_types.Event{}, err
		}
		if present {
			return replay(value, fingerprint)
		}
	}
	var result *ohrats_rc_events_types.Event
	err = transact(func(uint64) ([]storageTypes.Change, error) {
		if idempotency != nil {
			_, present, err := get(IdempotencyBucket, idempotency)
			if err != nil {
				return nil, err
			}
			if present {
				return nil, errors.New("idempotency retry raced; retry request")
			}
		}
		next, present, err := get(MetaBucket, []byte(nextCursorKey))
		if err != nil {
			return nil, err
		}
		current, err := readU64(next, present)
		if err != nil {
			return nil, err
		}
		if current == math.MaxUint64 {
			return nil, errors.New("event cursor exhausted")
		}
		cursor := current + 1
		stored := StoredEvent{
			Cursor:         cursor,
			Kind:           request.Kind,
			OccurredAtMs:   request.OccurredAtMs,
			ActorAccountID: pointerFromOption(request.ActorAccountId),
			Detail:         storeDetail(request.Detail),
		}
		encoded, err := encodeStoredEvent(stored)
		if err != nil {
			return nil, err
		}
		maximum, err := Policy()
		if err != nil {
			return nil, err
		}
		entries, err := scan(EventsBucket)
		if err != nil {
			return nil, err
		}
		prune := len(entries) + 1 - int(maximum)
		if prune < 0 {
			prune = 0
		}
		pruned := make([][]byte, 0, prune)
		changes := make([]storageTypes.Change, 0, prune+3)
		for _, entry := range entries[:prune] {
			pruned = append(pruned, entry.Key)
			changes = append(changes, delete(EventsBucket, entry.Key))
		}
		extra, err := pruneIdempotency(pruned)
		if err != nil {
			return nil, err
		}
		changes = append(changes, extra...)
		changes = append(changes, put(EventsBucket, cursorKey(cursor), encoded))
		changes = append(changes, put(MetaBucket, []byte(nextCursorKey), be64(cursor)))
		if idempotency != nil {
			value, err := encodeIdempotency(StoredIdempotency{
				Cursor:        cursor,
				RequestSHA256: fingerprint,
			})
			if err != nil {
				return nil, err
			}
			changes = append(changes, put(IdempotencyBucket, idempotency, value))
		}
		event, err := stored.Wire()
		if err != nil {
			return nil, err
		}
		result = &event
		return changes, nil
	})
	if err != nil {
		return ohrats_rc_events_types.Event{}, err
	}
	if result == nil {
		return ohrats_rc_events_types.Event{}, errors.New("event append produced no event")
	}
	return *result, nil
}

func replay(value []byte, fingerprint []byte) (ohrats_rc_events_types.Event, error) {
	stored, err := decodeIdempotency(value)
	if err != nil {
		return ohrats_rc_events_types.Event{}, err
	}
	event, err := load(stored.Cursor)
	if err != nil {
		return ohrats_rc_events_types.Event{}, err
	}
	if !bytes.Equal(stored.RequestSHA256, fingerprint) {
		return ohrats_rc_events_types.Event{}, errors.New("idempotency key was already used for a different event")
	}
	return event, nil
}

func requestFingerprint(request ohrats_rc_events_types.AppendRequest) ([]byte, error) {
	encoded, err := encodeStoredEvent(StoredEvent{
		Cursor:         0,
		Kind:           request.Kind,
		OccurredAtMs:   request.OccurredAtMs,
		ActorAccountID: pointerFromOption(request.ActorAccountId),
		Detail:         storeDetail(request.Detail),
	})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(encoded)
	return digest[:], nil
}

func load(cursor uint64) (ohrats_rc_events_types.Event, error) {
	value, present, err := get(EventsBucket, cursorKey(cursor))
	if err != nil {
		return ohrats_rc_events_types.Event{}, err
	}
	if !present {
		return ohrats_rc_events_types.Event{}, errors.New("idempotent event expired from retention")
	}
	stored, err := decodeStoredEvent(value)
	if err != nil {
		return ohrats_rc_events_types.Event{}, err
	}
	return stored.Wire()
}

func idempotencyKey(value string) []byte {
	digest := sha256.Sum256([]byte(value))
	return digest[:]
}

func storeDetail(value ohrats_rc_events_types.Detail) StoredDetail {
	switch value.Tag() {
	case ohrats_rc_events_types.Detail_Account:
		detail := value.Account()
		return StoredDetail{
			Tag:       ohrats_rc_events_types.Detail_Account,
			AccountID: detail.AccountId,
			Name:      pointerFromOption(detail.DisplayName),
		}
	case ohrats_rc_events_types.Detail_Workspace:
		detail := value.Workspace()
		return StoredDetail{
			Tag:         ohrats_rc_events_types.Detail_Workspace,
			WorkspaceID: detail.WorkspaceId,
			Name:        pointerFromOption(detail.Name),
		}
	case ohrats_rc_events_types.Detail_Membership:
		detail := value.Membership()
		return StoredDetail{
			Tag:         ohrats_rc_events_types.Detail_Membership,
			WorkspaceID: detail.WorkspaceId,
			AccountID:   detail.AccountId,
			Name:        pointerFromOption(detail.Role),
		}
	case ohrats_rc_events_types.Detail_Invitation:
		detail := value.Invitation()
		return StoredDetail{
			Tag:          ohrats_rc_events_types.Detail_Invitation,
			WorkspaceID:  detail.WorkspaceId,
			InvitationID: detail.InvitationId,
		}
	case ohrats_rc_events_types.Detail_Device:
		detail := value.Device()
		return StoredDetail{
			Tag:         ohrats_rc_events_types.Detail_Device,
			WorkspaceID: detail.WorkspaceId,
			DeviceID:    detail.DeviceId,
			Name:        pointerFromOption(detail.Name),
		}
	default:
		return StoredDetail{}
	}
}

func be64(value uint64) []byte {
	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, value)
	return encoded
}
