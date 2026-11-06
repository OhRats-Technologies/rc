package events

import (
	"bytes"
	"encoding/json"
	"errors"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_events_types"
)

// StoredEvent is the persisted lifecycle event. Its JSON encoding keeps the
// externally tagged serde representation used by earlier releases so existing
// durable state stays readable.
type StoredEvent struct {
	Cursor         uint64
	Kind           uint8
	OccurredAtMs   uint64
	ActorAccountID *string
	Detail         StoredDetail
}

// StoredDetail mirrors the stored detail enum. Fields are relevant per tag:
// account (AccountID, Name), workspace (WorkspaceID, Name), membership
// (WorkspaceID, AccountID, Name), invitation (WorkspaceID, InvitationID) and
// device (WorkspaceID, DeviceID, Name).
type StoredDetail struct {
	Tag          uint8
	WorkspaceID  string
	AccountID    string
	DeviceID     string
	InvitationID string
	Name         *string
}

// StoredIdempotency records the cursor created for a request key together with
// the request fingerprint that produced it.
type StoredIdempotency struct {
	Cursor        uint64
	RequestSHA256 []byte
}

type storedEventJSON struct {
	Cursor         uint64       `json:"cursor"`
	Kind           uint8        `json:"kind"`
	OccurredAtMs   uint64       `json:"occurred_at_ms"`
	ActorAccountID *string      `json:"actor_account_id"`
	Detail         StoredDetail `json:"detail"`
}

// byteArray encodes byte slices as JSON number arrays, matching serde's Vec<u8>.
type byteArray []byte

func encodeJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func encodeStoredEvent(value StoredEvent) ([]byte, error) {
	return encodeJSON(storedEventJSON{
		Cursor:         value.Cursor,
		Kind:           value.Kind,
		OccurredAtMs:   value.OccurredAtMs,
		ActorAccountID: value.ActorAccountID,
		Detail:         value.Detail,
	})
}

func decodeStoredEvent(value []byte) (StoredEvent, error) {
	var wire storedEventJSON
	if err := json.Unmarshal(value, &wire); err != nil {
		return StoredEvent{}, err
	}
	return StoredEvent{
		Cursor:         wire.Cursor,
		Kind:           wire.Kind,
		OccurredAtMs:   wire.OccurredAtMs,
		ActorAccountID: wire.ActorAccountID,
		Detail:         wire.Detail,
	}, nil
}

func encodeIdempotency(value StoredIdempotency) ([]byte, error) {
	return encodeJSON(struct {
		Cursor        uint64    `json:"cursor"`
		RequestSHA256 byteArray `json:"request_sha256"`
	}{Cursor: value.Cursor, RequestSHA256: byteArray(value.RequestSHA256)})
}

func decodeIdempotency(value []byte) (StoredIdempotency, error) {
	var wire struct {
		Cursor        uint64    `json:"cursor"`
		RequestSHA256 byteArray `json:"request_sha256"`
	}
	if err := json.Unmarshal(value, &wire); err != nil {
		return StoredIdempotency{}, err
	}
	return StoredIdempotency{Cursor: wire.Cursor, RequestSHA256: wire.RequestSHA256}, nil
}

func (value byteArray) MarshalJSON() ([]byte, error) {
	numbers := make([]uint32, len(value))
	for index, element := range value {
		numbers[index] = uint32(element)
	}
	return encodeJSON(numbers)
}

func (value *byteArray) UnmarshalJSON(data []byte) error {
	var numbers []uint32
	if err := json.Unmarshal(data, &numbers); err != nil {
		return err
	}
	result := make(byteArray, len(numbers))
	for index, element := range numbers {
		if element > 0xff {
			return errors.New("invalid stored event digest")
		}
		result[index] = byte(element)
	}
	*value = result
	return nil
}

func decodeString(data json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return "", err
	}
	return value, nil
}

func decodeName(data json.RawMessage) (*string, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, nil
	}
	value, err := decodeString(data)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

// Wire converts a stored event to its component ABI representation.
func (value StoredEvent) Wire() (ohrats_rc_events_types.Event, error) {
	kind, err := decodeKind(value.Kind)
	if err != nil {
		return ohrats_rc_events_types.Event{}, err
	}
	detail, err := value.Detail.Wire()
	if err != nil {
		return ohrats_rc_events_types.Event{}, err
	}
	return ohrats_rc_events_types.Event{
		Cursor:         value.Cursor,
		Kind:           kind,
		OccurredAtMs:   value.OccurredAtMs,
		ActorAccountId: optionFromPointer(value.ActorAccountID),
		Detail:         detail,
	}, nil
}

// Wire converts a stored detail to its component ABI representation.
func (value StoredDetail) Wire() (ohrats_rc_events_types.Detail, error) {
	switch value.Tag {
	case ohrats_rc_events_types.Detail_Account:
		return ohrats_rc_events_types.MakeDetailAccount(ohrats_rc_events_types.AccountDetail{
			AccountId:   value.AccountID,
			DisplayName: optionFromPointer(value.Name),
		}), nil
	case ohrats_rc_events_types.Detail_Workspace:
		return ohrats_rc_events_types.MakeDetailWorkspace(ohrats_rc_events_types.WorkspaceDetail{
			WorkspaceId: value.WorkspaceID,
			Name:        optionFromPointer(value.Name),
		}), nil
	case ohrats_rc_events_types.Detail_Membership:
		return ohrats_rc_events_types.MakeDetailMembership(ohrats_rc_events_types.MembershipDetail{
			WorkspaceId: value.WorkspaceID,
			AccountId:   value.AccountID,
			Role:        optionFromPointer(value.Name),
		}), nil
	case ohrats_rc_events_types.Detail_Invitation:
		return ohrats_rc_events_types.MakeDetailInvitation(ohrats_rc_events_types.InvitationDetail{
			WorkspaceId:  value.WorkspaceID,
			InvitationId: value.InvitationID,
		}), nil
	case ohrats_rc_events_types.Detail_Device:
		return ohrats_rc_events_types.MakeDetailDevice(ohrats_rc_events_types.DeviceDetail{
			WorkspaceId: value.WorkspaceID,
			DeviceId:    value.DeviceID,
			Name:        optionFromPointer(value.Name),
		}), nil
	default:
		return ohrats_rc_events_types.Detail{}, errors.New("invalid stored event detail")
	}
}

func decodeKind(value uint8) (ohrats_rc_events_types.LifecycleKind, error) {
	if value > ohrats_rc_events_types.LifecycleKind_DeviceRevoked {
		return 0, errors.New("invalid stored event kind")
	}
	return value, nil
}

func optionFromPointer(value *string) witTypes.Option[string] {
	if value == nil {
		return witTypes.None[string]()
	}
	return witTypes.Some(*value)
}

func pointerFromOption(value witTypes.Option[string]) *string {
	if value.IsNone() {
		return nil
	}
	text := value.Some()
	return &text
}
