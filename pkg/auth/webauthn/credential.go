package webauthn

// Credential is validated credential state persisted by the relying party.
type Credential struct {
	ID             []byte
	Algorithm      string
	PublicKeyCOSE  []byte
	SignCount      uint32
	BackupEligible bool
	BackupState    bool
}

// newChallenge validates a server-side ceremony challenge.
func newChallenge(value []byte) ([]byte, error) {
	if len(value) < 16 || len(value) > 1024 {
		return nil, ErrInvalidChallenge
	}
	return copyBytes(value), nil
}

// decodeCredential validates stored credential state before a ceremony.
func decodeCredential(value Credential) (Credential, error) {
	key, err := fromCOSEKey(value.PublicKeyCOSE)
	if err != nil {
		return Credential{}, err
	}
	if value.BackupState && !value.BackupEligible {
		return Credential{}, ErrInvalidBackupState
	}
	if len(value.ID) == 0 {
		return Credential{}, ErrEmptyCredentialID
	}
	if len(value.ID) > maxCredentialIDBytes {
		return Credential{}, ErrCredentialIDTooLarge
	}
	value.ID = copyBytes(value.ID)
	value.Algorithm = Algorithm
	value.PublicKeyCOSE = key.coseKey()
	return value, nil
}

func newCredential(
	id []byte,
	key publicKey,
	signCount uint32,
	backupEligible bool,
	backupState bool,
) (Credential, error) {
	if len(id) == 0 {
		return Credential{}, ErrEmptyCredentialID
	}
	if len(id) > maxCredentialIDBytes {
		return Credential{}, ErrCredentialIDTooLarge
	}
	if backupState && !backupEligible {
		return Credential{}, ErrInvalidBackupState
	}
	return Credential{
		ID:             copyBytes(id),
		Algorithm:      Algorithm,
		PublicKeyCOSE:  key.coseKey(),
		SignCount:      signCount,
		BackupEligible: backupEligible,
		BackupState:    backupState,
	}, nil
}
