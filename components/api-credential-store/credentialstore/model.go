package credentialstore

import (
	"encoding/json"
	"errors"
	"strconv"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_api_credentials_types"
)

// Durable store buckets. The names are part of the on-disk contract and must
// not change without a migration.
const (
	credentialsBucket    = "api-credentials"
	authorizationsBucket = "api-cli-authorizations"
	noncesBucket         = "api-request-nonces"
)

const (
	credentialKindAPI = 0
	credentialKindCLI = 1

	cliRequestTTLMs = 10 * 60 * 1000
	cliPollInterval = 2
)

// The durable representation mirrors the previous serde model: snake_case
// field names, byte vectors as number arrays, and absent options as null.
type storedCredential struct {
	Id           string    `json:"id"`
	UserId       string    `json:"user_id"`
	Kind         uint8     `json:"kind"`
	Name         string    `json:"name"`
	PublicKey    string    `json:"public_key"`
	Scopes       byteArray `json:"scopes"`
	CreatedAtMs  uint64    `json:"created_at_ms"`
	ExpiresAtMs  uint64    `json:"expires_at_ms"`
	LastUsedAtMs *uint64   `json:"last_used_at_ms"`
	RevokedAtMs  *uint64   `json:"revoked_at_ms"`
}

type pendingCLI struct {
	RequestId      string    `json:"request_id"`
	ClientId       string    `json:"client_id"`
	PublicKey      string    `json:"public_key"`
	Lifetime       *uint8    `json:"lifetime"`
	DeviceCodeHash byteArray `json:"device_code_hash"`
	UserCodeHash   byteArray `json:"user_code_hash"`
	CreatedAtMs    uint64    `json:"created_at_ms"`
	ExpiresAtMs    uint64    `json:"expires_at_ms"`
	ApprovedAtMs   *uint64   `json:"approved_at_ms"`
	ExchangedAtMs  *uint64   `json:"exchanged_at_ms"`
}

func (value storedCredential) view() ohrats_rc_api_credentials_types.Credential {
	kind := ohrats_rc_api_credentials_types.Kind_Api
	if value.Kind == credentialKindCLI {
		kind = ohrats_rc_api_credentials_types.Kind_Cli
	}
	storedScopes := make([]ohrats_rc_api_credentials_types.Scope, 0, len(value.Scopes))
	for _, scope := range value.Scopes {
		storedScopes = append(storedScopes, viewScope(scope))
	}
	credential := ohrats_rc_api_credentials_types.Credential{
		Id:           value.Id,
		UserId:       value.UserId,
		Kind:         kind,
		Name:         value.Name,
		PublicKey:    value.PublicKey,
		Scopes:       storedScopes,
		CreatedAtMs:  value.CreatedAtMs,
		ExpiresAtMs:  value.ExpiresAtMs,
		LastUsedAtMs: witTypes.None[uint64](),
		RevokedAtMs:  witTypes.None[uint64](),
	}
	if value.LastUsedAtMs != nil {
		credential.LastUsedAtMs = witTypes.Some(*value.LastUsedAtMs)
	}
	if value.RevokedAtMs != nil {
		credential.RevokedAtMs = witTypes.Some(*value.RevokedAtMs)
	}
	return credential
}

func viewScope(value uint8) ohrats_rc_api_credentials_types.Scope {
	switch value {
	case ohrats_rc_api_credentials_types.Scope_Execute:
		return ohrats_rc_api_credentials_types.Scope_Execute
	case ohrats_rc_api_credentials_types.Scope_ManageDevices:
		return ohrats_rc_api_credentials_types.Scope_ManageDevices
	case ohrats_rc_api_credentials_types.Scope_ManageWorkspaces:
		return ohrats_rc_api_credentials_types.Scope_ManageWorkspaces
	default:
		return ohrats_rc_api_credentials_types.Scope_Read
	}
}

func storedScopes(value []ohrats_rc_api_credentials_types.Scope) byteArray {
	result := make(byteArray, 0, len(value))
	for _, scope := range value {
		result = append(result, scope)
	}
	return result
}

// storedLifetime keeps the requested lifetime so approval can start the
// credential lifetime from the moment the human approved it.
func storedLifetime(value witTypes.Option[ohrats_rc_api_credentials_types.Lifetime]) *uint8 {
	if value.IsNone() {
		return nil
	}
	stored := value.Some()
	return &stored
}

func lifetimeFromStored(value *uint8) witTypes.Option[ohrats_rc_api_credentials_types.Lifetime] {
	if value == nil {
		return witTypes.None[ohrats_rc_api_credentials_types.Lifetime]()
	}
	switch *value {
	case ohrats_rc_api_credentials_types.Lifetime_OneHour:
		return witTypes.Some(ohrats_rc_api_credentials_types.Lifetime_OneHour)
	case ohrats_rc_api_credentials_types.Lifetime_OneDay:
		return witTypes.Some(ohrats_rc_api_credentials_types.Lifetime_OneDay)
	case ohrats_rc_api_credentials_types.Lifetime_SevenDays:
		return witTypes.Some(ohrats_rc_api_credentials_types.Lifetime_SevenDays)
	case ohrats_rc_api_credentials_types.Lifetime_ThirtyDays:
		return witTypes.Some(ohrats_rc_api_credentials_types.Lifetime_ThirtyDays)
	case ohrats_rc_api_credentials_types.Lifetime_NinetyDays:
		return witTypes.Some(ohrats_rc_api_credentials_types.Lifetime_NinetyDays)
	case ohrats_rc_api_credentials_types.Lifetime_OneEightyDays:
		return witTypes.Some(ohrats_rc_api_credentials_types.Lifetime_OneEightyDays)
	case ohrats_rc_api_credentials_types.Lifetime_OneYear:
		return witTypes.Some(ohrats_rc_api_credentials_types.Lifetime_OneYear)
	default:
		return witTypes.Some(ohrats_rc_api_credentials_types.Lifetime_Never)
	}
}

// byteArray encodes a byte vector as a JSON number array, matching serde's
// representation of Vec<u8>.
type byteArray []uint8

func (value byteArray) MarshalJSON() ([]byte, error) {
	out := make([]byte, 0, 2+len(value)*4)
	out = append(out, '[')
	for index, item := range value {
		if index > 0 {
			out = append(out, ',')
		}
		out = strconv.AppendUint(out, uint64(item), 10)
	}
	out = append(out, ']')
	return out, nil
}

func (value *byteArray) UnmarshalJSON(data []byte) error {
	var numbers []uint16
	if err := json.Unmarshal(data, &numbers); err != nil {
		return err
	}
	result := make(byteArray, len(numbers))
	for index, number := range numbers {
		if number > 0xff {
			return errors.New("invalid stored byte array")
		}
		result[index] = byte(number)
	}
	*value = result
	return nil
}

func encodeCredential(value storedCredential) ([]byte, error) {
	if value.Scopes == nil {
		value.Scopes = byteArray{}
	}
	return encode(value)
}

func encodeAuthorization(value pendingCLI) ([]byte, error) {
	if value.DeviceCodeHash == nil {
		value.DeviceCodeHash = byteArray{}
	}
	if value.UserCodeHash == nil {
		value.UserCodeHash = byteArray{}
	}
	return encode(value)
}

func encode(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New(err.Error())
	}
	return encoded, nil
}

func decodeCredential(value []byte) (storedCredential, error) {
	var credential storedCredential
	if err := json.Unmarshal(value, &credential); err != nil {
		return storedCredential{}, errors.New("invalid credential")
	}
	return credential, nil
}

func decodeAuthorization(value []byte) (pendingCLI, error) {
	var pending pendingCLI
	if err := json.Unmarshal(value, &pending); err != nil {
		return pendingCLI{}, errors.New("invalid CLI authorization")
	}
	return pending, nil
}
