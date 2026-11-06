package credentialstore

import (
	"errors"
	"math"
	"sort"
	"strconv"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_api_credentials_types"
	"wit_component/ohrats_rc_identity_types"
)

const maxAPIKeys = 10

// CreateAPI registers a client-side generated API signing key after the human
// administrator proof for the `api-credential.create` operation is consumed.
func CreateAPI(
	admin ohrats_rc_identity_types.HumanAuthorization,
	id string,
	name string,
	publicKeyValue string,
	requested []ohrats_rc_api_credentials_types.Scope,
	life witTypes.Option[ohrats_rc_api_credentials_types.Lifetime],
) (ohrats_rc_api_credentials_types.Credential, error) {
	claim, err := checkAdmin(admin, "api-credential.create")
	if err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, err
	}
	if err := identifier(id, "credential id"); err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, err
	}
	if err := text(name, "credential name", maxName); err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, err
	}
	if err := publicKey(publicKeyValue); err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, err
	}
	requested = scopes(requested)
	_, present, err := getValue(credentialsBucket, []byte(id))
	if err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, err
	}
	if present {
		return ohrats_rc_api_credentials_types.Credential{}, errors.New("API credential already exists")
	}
	existing, err := List(claim.UserId)
	if err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, err
	}
	count := 0
	for _, credential := range existing {
		if credential.Kind == ohrats_rc_api_credentials_types.Kind_Api {
			count++
		}
	}
	if count >= maxAPIKeys {
		return ohrats_rc_api_credentials_types.Credential{}, errors.New("API key limit reached (10)")
	}
	value := storedCredential{
		Id:          id,
		UserId:      claim.UserId,
		Kind:        credentialKindAPI,
		Name:        name,
		PublicKey:   publicKeyValue,
		Scopes:      storedScopes(requested),
		CreatedAtMs: claim.ConsumedAtMs,
		ExpiresAtMs: lifetime(life, claim.ConsumedAtMs),
	}
	encoded, err := encodeCredential(value)
	if err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, errors.New("encode credential")
	}
	if err := putValue(credentialsBucket, []byte(id), encoded); err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, err
	}
	return value.view(), nil
}

// List returns every credential owned by the user, ordered by id.
func List(userId string) ([]ohrats_rc_api_credentials_types.Credential, error) {
	if err := identifier(userId, "user id"); err != nil {
		return nil, err
	}
	entries, err := scanBucket(credentialsBucket)
	if err != nil {
		return nil, err
	}
	values := make([]ohrats_rc_api_credentials_types.Credential, 0, len(entries))
	for _, entry := range entries {
		value, err := decodeCredential(entry.Value)
		if err != nil {
			continue
		}
		if value.UserId != userId {
			continue
		}
		values = append(values, value.view())
	}
	sort.Slice(values, func(first int, second int) bool {
		return values[first].Id < values[second].Id
	})
	return values, nil
}

// Get returns one credential without exposing revoked state filtering.
func Get(id string) (witTypes.Option[ohrats_rc_api_credentials_types.Credential], error) {
	if err := identifier(id, "credential id"); err != nil {
		return witTypes.None[ohrats_rc_api_credentials_types.Credential](), err
	}
	encoded, present, err := getValue(credentialsBucket, []byte(id))
	if err != nil {
		return witTypes.None[ohrats_rc_api_credentials_types.Credential](), err
	}
	if !present {
		return witTypes.None[ohrats_rc_api_credentials_types.Credential](), nil
	}
	value, err := decodeCredential(encoded)
	if err != nil {
		return witTypes.None[ohrats_rc_api_credentials_types.Credential](), err
	}
	return witTypes.Some(value.view()), nil
}

// Revoke marks one owned credential revoked. Unknown or foreign credentials
// report false instead of leaking their existence.
func Revoke(admin ohrats_rc_identity_types.HumanAuthorization, id string) (bool, error) {
	claim, err := checkAdmin(admin, "api-credential.revoke")
	if err != nil {
		return false, err
	}
	value, present, err := load(id)
	if err != nil {
		return false, err
	}
	if !present {
		return false, nil
	}
	if value.UserId != claim.UserId || value.RevokedAtMs != nil {
		return false, nil
	}
	revokedAt := claim.ConsumedAtMs
	value.RevokedAtMs = &revokedAt
	encoded, err := encodeCredential(value)
	if err != nil {
		return false, errors.New("encode credential")
	}
	if err := putValue(credentialsBucket, []byte(id), encoded); err != nil {
		return false, err
	}
	return true, nil
}

// Verify authenticates one signed API request and records its nonce so the
// same request cannot be replayed.
func Verify(request ohrats_rc_api_credentials_types.Request, nowMs uint64) (ohrats_rc_api_credentials_types.Verified, error) {
	timestamp, err := strconv.ParseInt(request.TimestampSeconds, 10, 64)
	if err != nil {
		return ohrats_rc_api_credentials_types.Verified{}, errors.New("invalid timestamp")
	}
	if nowMs/1000 > math.MaxInt64 {
		return ohrats_rc_api_credentials_types.Verified{}, errors.New("invalid clock")
	}
	now := int64(nowMs / 1000)
	if clockSkew(now, timestamp) > 60 {
		return ohrats_rc_api_credentials_types.Verified{}, errors.New("expired client authentication")
	}
	nonceKey := request.KeyId + ":" + hexLower(hash(request.Nonce))
	for attempt := 0; attempt < retries; attempt++ {
		// Capture the revision before every read. If another verifier records
		// this nonce while we validate, our commit must conflict and retry the
		// nonce lookup instead of accepting the stale absence.
		expected, err := revision()
		if err != nil {
			return ohrats_rc_api_credentials_types.Verified{}, err
		}
		value, present, err := load(request.KeyId)
		if err != nil {
			return ohrats_rc_api_credentials_types.Verified{}, err
		}
		if !present {
			return ohrats_rc_api_credentials_types.Verified{}, errors.New("client authentication rejected")
		}
		if value.RevokedAtMs != nil || (value.ExpiresAtMs != 0 && value.ExpiresAtMs <= nowMs) {
			return ohrats_rc_api_credentials_types.Verified{}, errors.New("client authentication rejected")
		}
		if err := verifyRequest(request, value.PublicKey); err != nil {
			return ohrats_rc_api_credentials_types.Verified{}, err
		}
		storedNonce, recorded, err := getValue(noncesBucket, []byte(nonceKey))
		if err != nil {
			return ohrats_rc_api_credentials_types.Verified{}, err
		}
		if expires, ok := nonceExpiry(storedNonce); recorded && ok && expires > nowMs {
			return ohrats_rc_api_credentials_types.Verified{}, errors.New("replayed client request")
		}
		lastUsed := nowMs
		value.LastUsedAtMs = &lastUsed
		changes, err := expiredNonceDeletions(nowMs, nonceKey)
		if err != nil {
			return ohrats_rc_api_credentials_types.Verified{}, err
		}
		encoded, err := encodeCredential(value)
		if err != nil {
			return ohrats_rc_api_credentials_types.Verified{}, errors.New("encode credential")
		}
		changes = append(changes,
			putChange(credentialsBucket, []byte(request.KeyId), encoded),
			putChange(noncesBucket, []byte(nonceKey), []byte(strconv.FormatUint(nonceExpiryValue(nowMs), 10))),
		)
		committed, err := commitOnce(expected, changes)
		if err != nil {
			return ohrats_rc_api_credentials_types.Verified{}, err
		}
		if !committed {
			continue
		}
		output := value.view()
		return ohrats_rc_api_credentials_types.Verified{
			CredentialId: output.Id,
			UserId:       output.UserId,
			Kind:         output.Kind,
			Scopes:       output.Scopes,
		}, nil
	}
	return ohrats_rc_api_credentials_types.Verified{}, errors.New("API credential state changed too frequently")
}

func load(id string) (storedCredential, bool, error) {
	if err := identifier(id, "credential id"); err != nil {
		return storedCredential{}, false, err
	}
	encoded, present, err := getValue(credentialsBucket, []byte(id))
	if err != nil {
		return storedCredential{}, false, err
	}
	if !present {
		return storedCredential{}, false, nil
	}
	value, err := decodeCredential(encoded)
	if err != nil {
		return storedCredential{}, false, err
	}
	return value, true, nil
}
