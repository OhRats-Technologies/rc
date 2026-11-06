package identitystore

import (
	"bytes"
	"errors"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_identity_credentials"
	"wit_component/ohrats_rc_identity_types"
	"wit_component/ohrats_rc_webauthn_types"
)

const (
	passkeyBucket      = "passkeys"
	passkeyIndexBucket = "passkeys-by-credential"
	passkeyIDBytes     = 16
)

type storedCredential struct {
	Id             byteArray `json:"id"`
	Algorithm      string    `json:"algorithm"`
	PublicKeyCose  byteArray `json:"public_key_cose"`
	SignCount      uint32    `json:"sign_count"`
	BackupEligible bool      `json:"backup_eligible"`
	BackupState    bool      `json:"backup_state"`
}

type storedPasskey struct {
	Id           string           `json:"id"`
	UserId       string           `json:"user_id"`
	Name         string           `json:"name"`
	Credential   storedCredential `json:"credential"`
	CreatedAtMs  uint64           `json:"created_at_ms"`
	LastUsedAtMs *uint64          `json:"last_used_at_ms"`
}

func (value storedPasskey) view() ohrats_rc_identity_credentials.Passkey {
	passkey := ohrats_rc_identity_credentials.Passkey{
		Id:           value.Id,
		UserId:       value.UserId,
		Name:         value.Name,
		Credential:   value.Credential.view(),
		CreatedAtMs:  value.CreatedAtMs,
		LastUsedAtMs: witTypes.None[uint64](),
	}
	if value.LastUsedAtMs != nil {
		passkey.LastUsedAtMs = witTypes.Some(*value.LastUsedAtMs)
	}
	return passkey
}

func (value storedCredential) view() ohrats_rc_webauthn_types.StoredCredential {
	return ohrats_rc_webauthn_types.StoredCredential{
		Id:             value.Id,
		Algorithm:      value.Algorithm,
		PublicKeyCose:  value.PublicKeyCose,
		SignCount:      value.SignCount,
		BackupEligible: value.BackupEligible,
		BackupState:    value.BackupState,
	}
}

func storeCredential(value ohrats_rc_webauthn_types.StoredCredential) storedCredential {
	return storedCredential{
		Id:             emptyByteArray(value.Id),
		Algorithm:      value.Algorithm,
		PublicKeyCose:  emptyByteArray(value.PublicKeyCose),
		SignCount:      value.SignCount,
		BackupEligible: value.BackupEligible,
		BackupState:    value.BackupState,
	}
}

func (value storedPasskey) encoded() ([]byte, error) {
	value.Credential.Id = emptyByteArray(value.Credential.Id)
	value.Credential.PublicKeyCose = emptyByteArray(value.Credential.PublicKeyCose)
	return encode(value)
}

// CreateUserWithPasskey creates a passwordless user and its first passkey in
// one transactional write.
func CreateUserWithPasskey(
	userId string,
	displayName string,
	passkeyName string,
	credential ohrats_rc_webauthn_types.StoredCredential,
) (ohrats_rc_identity_types.User, error) {
	user, userKey, userValue, err := prepareUser(userId, displayName)
	if err != nil {
		return ohrats_rc_identity_types.User{}, err
	}
	passkey, passkeyKey, passkeyValue, indexKey, err := preparePasskey(userId, passkeyName, credential)
	if err != nil {
		return ohrats_rc_identity_types.User{}, err
	}
	err = insertMany([]Put{
		{Bucket: userBucket, Key: userKey, Value: userValue},
		{Bucket: passkeyBucket, Key: passkeyKey, Value: passkeyValue},
		{Bucket: passkeyIndexBucket, Key: indexKey, Value: []byte(passkey.Id)},
	})
	if err != nil {
		return ohrats_rc_identity_types.User{}, err
	}
	return user, nil
}

func AddPasskey(
	userId string,
	name string,
	credential ohrats_rc_webauthn_types.StoredCredential,
) (ohrats_rc_identity_credentials.Passkey, error) {
	_, found, err := getUser(userId)
	if err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, err
	}
	if !found {
		return ohrats_rc_identity_credentials.Passkey{}, errors.New("user not found")
	}
	passkey, key, value, indexKey, err := preparePasskey(userId, name, credential)
	if err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, err
	}
	err = insertMany([]Put{
		{Bucket: passkeyBucket, Key: key, Value: value},
		{Bucket: passkeyIndexBucket, Key: indexKey, Value: []byte(passkey.Id)},
	})
	if err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, err
	}
	return passkey, nil
}

func GetPasskeyByCredentialID(id []byte) (witTypes.Option[ohrats_rc_identity_credentials.Passkey], error) {
	if err := validCredentialID(id); err != nil {
		return witTypes.None[ohrats_rc_identity_credentials.Passkey](), err
	}
	internal, found, err := getValue(passkeyIndexBucket, credentialKey(id))
	if err != nil || !found {
		return witTypes.None[ohrats_rc_identity_credentials.Passkey](), err
	}
	passkeyID, err := utf8String(internal)
	if err != nil {
		return witTypes.None[ohrats_rc_identity_credentials.Passkey](), err
	}
	return getPasskey(passkeyID)
}

func AllPasskeys(userId witTypes.Option[string]) ([]ohrats_rc_identity_credentials.Passkey, error) {
	filter := ""
	hasFilter := false
	if userId.IsSome() {
		filter = userId.Some()
		hasFilter = true
		if err := validID(filter, "user id"); err != nil {
			return nil, err
		}
	}
	entries, err := scanAll(passkeyBucket)
	if err != nil {
		return nil, err
	}
	passkeys := make([]ohrats_rc_identity_credentials.Passkey, 0, len(entries))
	for _, entry := range entries {
		stored, err := decode[storedPasskey](entry.Value)
		if err != nil {
			return nil, err
		}
		if hasFilter && stored.UserId != filter {
			continue
		}
		passkeys = append(passkeys, stored.view())
	}
	return passkeys, nil
}

func UpdatePasskey(
	id string,
	credential ohrats_rc_webauthn_types.StoredCredential,
	usedAtMs uint64,
) (ohrats_rc_identity_credentials.Passkey, error) {
	if err := validID(id, "passkey id"); err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, err
	}
	if err := validStoredCredential(credential.Id, credential.Algorithm, credential.PublicKeyCose); err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, err
	}
	value, found, err := getValue(passkeyBucket, []byte(id))
	if err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, err
	}
	if !found {
		return ohrats_rc_identity_credentials.Passkey{}, errors.New("passkey not found")
	}
	stored, err := decode[storedPasskey](value)
	if err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, err
	}
	if !bytes.Equal(stored.Credential.Id, credential.Id) {
		return ohrats_rc_identity_credentials.Passkey{}, errors.New("WebAuthn credential id cannot change")
	}
	stored.Credential = storeCredential(credential)
	used := usedAtMs
	if now := nowMs(); now > used {
		used = now
	}
	stored.LastUsedAtMs = &used
	encoded, err := stored.encoded()
	if err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, err
	}
	if err := replace(passkeyBucket, []byte(id), encoded); err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, err
	}
	return stored.view(), nil
}

func RemovePasskey(id string, userId string) (bool, error) {
	if err := validID(id, "passkey id"); err != nil {
		return false, err
	}
	if err := validID(userId, "user id"); err != nil {
		return false, err
	}
	value, found, err := getValue(passkeyBucket, []byte(id))
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	stored, err := decode[storedPasskey](value)
	if err != nil {
		return false, err
	}
	if stored.UserId != userId {
		return false, nil
	}
	return removeMany([]Delete{
		{Bucket: passkeyBucket, Key: []byte(id)},
		{Bucket: passkeyIndexBucket, Key: credentialKey(stored.Credential.Id)},
	})
}

func getPasskey(id string) (witTypes.Option[ohrats_rc_identity_credentials.Passkey], error) {
	if err := validID(id, "passkey id"); err != nil {
		return witTypes.None[ohrats_rc_identity_credentials.Passkey](), err
	}
	value, found, err := getValue(passkeyBucket, []byte(id))
	if err != nil || !found {
		return witTypes.None[ohrats_rc_identity_credentials.Passkey](), err
	}
	stored, err := decode[storedPasskey](value)
	if err != nil {
		return witTypes.None[ohrats_rc_identity_credentials.Passkey](), err
	}
	return witTypes.Some(stored.view()), nil
}

func preparePasskey(
	userId string,
	name string,
	credential ohrats_rc_webauthn_types.StoredCredential,
) (ohrats_rc_identity_credentials.Passkey, []byte, []byte, []byte, error) {
	if err := validID(userId, "user id"); err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, nil, nil, nil, err
	}
	if err := validPasskeyName(name); err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, nil, nil, nil, err
	}
	if err := validStoredCredential(credential.Id, credential.Algorithm, credential.PublicKeyCose); err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, nil, nil, nil, err
	}
	id, err := randomToken(passkeyIDBytes)
	if err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, nil, nil, nil, err
	}
	stored := storedPasskey{
		Id:           id,
		UserId:       userId,
		Name:         name,
		Credential:   storeCredential(credential),
		CreatedAtMs:  nowMs(),
		LastUsedAtMs: nil,
	}
	encoded, err := stored.encoded()
	if err != nil {
		return ohrats_rc_identity_credentials.Passkey{}, nil, nil, nil, err
	}
	index := credentialKey(stored.Credential.Id)
	return stored.view(), []byte(stored.Id), encoded, index, nil
}

func credentialKey(id []byte) []byte {
	return digest(id)
}
