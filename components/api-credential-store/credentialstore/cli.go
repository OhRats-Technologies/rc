package credentialstore

import (
	"errors"
	"fmt"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_api_credentials_types"
	"wit_component/ohrats_rc_identity_types"
)

// StartCLI registers a pending CLI login request. Only the hashes of the
// device and user codes are stored.
func StartCLI(
	clientId string,
	publicKeyValue string,
	life witTypes.Option[ohrats_rc_api_credentials_types.Lifetime],
	requestId string,
	deviceCode string,
	userCode string,
	nowMs uint64,
) (ohrats_rc_api_credentials_types.CliAuthorization, error) {
	if err := identifier(clientId, "CLI client id"); err != nil {
		return ohrats_rc_api_credentials_types.CliAuthorization{}, err
	}
	if err := identifier(requestId, "CLI request id"); err != nil {
		return ohrats_rc_api_credentials_types.CliAuthorization{}, err
	}
	if err := publicKey(publicKeyValue); err != nil {
		return ohrats_rc_api_credentials_types.CliAuthorization{}, err
	}
	if err := text(deviceCode, "CLI device code", maxCode); err != nil {
		return ohrats_rc_api_credentials_types.CliAuthorization{}, err
	}
	if err := text(userCode, "CLI user code", maxCode); err != nil {
		return ohrats_rc_api_credentials_types.CliAuthorization{}, err
	}
	_, present, err := getValue(authorizationsBucket, []byte(requestId))
	if err != nil {
		return ohrats_rc_api_credentials_types.CliAuthorization{}, err
	}
	if present {
		return ohrats_rc_api_credentials_types.CliAuthorization{}, errors.New("CLI authorization already exists")
	}
	pending := pendingCLI{
		RequestId:      requestId,
		ClientId:       clientId,
		PublicKey:      publicKeyValue,
		Lifetime:       storedLifetime(life),
		DeviceCodeHash: hash(deviceCode),
		UserCodeHash:   hash(userCode),
		CreatedAtMs:    nowMs,
		ExpiresAtMs:    requestExpiry(nowMs),
	}
	encoded, err := encodeAuthorization(pending)
	if err != nil {
		return ohrats_rc_api_credentials_types.CliAuthorization{}, err
	}
	if err := putValue(authorizationsBucket, []byte(requestId), encoded); err != nil {
		return ohrats_rc_api_credentials_types.CliAuthorization{}, err
	}
	return ohrats_rc_api_credentials_types.CliAuthorization{
		RequestId:       requestId,
		ClientId:        pending.ClientId,
		ExpiresAtMs:     pending.ExpiresAtMs,
		IntervalSeconds: cliPollInterval,
		VerificationUrl: fmt.Sprintf("/cli/login?code=%s", userCode),
	}, nil
}

// ApproveCLI binds an approved CLI request to a credential scoped read and
// execute. The browser client id must match the requesting CLI client.
func ApproveCLI(
	admin ohrats_rc_identity_types.HumanAuthorization,
	requestId string,
	userCode string,
	browserPublicKey string,
) (ohrats_rc_api_credentials_types.Credential, error) {
	claim, err := checkAdmin(admin, "api-credential.cli-approve")
	if err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, err
	}
	pending, present, err := loadAuthorization(requestId)
	if err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, err
	}
	if !present {
		return ohrats_rc_api_credentials_types.Credential{}, errors.New("CLI authorization expired")
	}
	userCodeHash := hash(userCode)
	if pending.ExpiresAtMs <= claim.ConsumedAtMs ||
		pending.ExchangedAtMs != nil ||
		pending.ApprovedAtMs != nil ||
		!equalBytes(pending.UserCodeHash, userCodeHash) ||
		pending.ClientId != claim.BrowserClientId ||
		pending.PublicKey != browserPublicKey {
		return ohrats_rc_api_credentials_types.Credential{}, errors.New("CLI authorization expired")
	}
	id := pending.ClientId
	_, exists, err := getValue(credentialsBucket, []byte(id))
	if err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, err
	}
	if exists {
		return ohrats_rc_api_credentials_types.Credential{}, errors.New("CLI credential already exists")
	}
	value := storedCredential{
		Id:        id,
		UserId:    claim.UserId,
		Kind:      credentialKindCLI,
		Name:      "RC CLI",
		PublicKey: pending.PublicKey,
		Scopes: byteArray{
			ohrats_rc_api_credentials_types.Scope_Read,
			ohrats_rc_api_credentials_types.Scope_Execute,
		},
		CreatedAtMs: claim.ConsumedAtMs,
		ExpiresAtMs: lifetime(lifetimeFromStored(pending.Lifetime), claim.ConsumedAtMs),
	}
	approvedAt := claim.ConsumedAtMs
	pending.ApprovedAtMs = &approvedAt
	encodedCredential, err := encodeCredential(value)
	if err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, errors.New("encode credential")
	}
	encodedAuthorization, err := encodeAuthorization(pending)
	if err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, err
	}
	if err := putValue(credentialsBucket, []byte(id), encodedCredential); err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, err
	}
	if err := putValue(authorizationsBucket, []byte(requestId), encodedAuthorization); err != nil {
		return ohrats_rc_api_credentials_types.Credential{}, err
	}
	return value.view(), nil
}

// PollCLI exchanges one approved request for the issued credential exactly
// once. An unapproved request reports none.
func PollCLI(requestId string, deviceCode string, nowMs uint64) (witTypes.Option[ohrats_rc_api_credentials_types.Credential], error) {
	none := witTypes.None[ohrats_rc_api_credentials_types.Credential]()
	pending, present, err := loadAuthorization(requestId)
	if err != nil {
		return none, err
	}
	if !present {
		return none, errors.New("CLI authorization expired")
	}
	if pending.ExpiresAtMs <= nowMs ||
		!equalBytes(pending.DeviceCodeHash, hash(deviceCode)) ||
		pending.ExchangedAtMs != nil {
		return none, errors.New("CLI authorization expired")
	}
	if pending.ApprovedAtMs == nil {
		return none, nil
	}
	exchangedAt := nowMs
	pending.ExchangedAtMs = &exchangedAt
	credential, err := Get(pending.ClientId)
	if err != nil {
		return none, err
	}
	if credential.IsNone() {
		return none, errors.New("CLI credential expired")
	}
	encoded, err := encodeAuthorization(pending)
	if err != nil {
		return none, err
	}
	if err := putValue(authorizationsBucket, []byte(requestId), encoded); err != nil {
		return none, err
	}
	return credential, nil
}

// RevokeCLI revokes the CLI credential with the normal credential policy.
func RevokeCLI(admin ohrats_rc_identity_types.HumanAuthorization, id string) (bool, error) {
	return Revoke(admin, id)
}

func loadAuthorization(id string) (pendingCLI, bool, error) {
	if err := identifier(id, "CLI request id"); err != nil {
		return pendingCLI{}, false, err
	}
	encoded, present, err := getValue(authorizationsBucket, []byte(id))
	if err != nil {
		return pendingCLI{}, false, err
	}
	if !present {
		return pendingCLI{}, false, nil
	}
	pending, err := decodeAuthorization(encoded)
	if err != nil {
		return pendingCLI{}, false, err
	}
	return pending, true, nil
}

func requestExpiry(nowMs uint64) uint64 {
	if nowMs > ^uint64(0)-cliRequestTTLMs {
		return ^uint64(0)
	}
	return nowMs + cliRequestTTLMs
}

func equalBytes(first []byte, second []byte) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}
