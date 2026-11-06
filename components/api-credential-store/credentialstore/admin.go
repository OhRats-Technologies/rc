package credentialstore

import (
	"errors"

	"wit_component/ohrats_rc_identity_admin_consumer"
	"wit_component/ohrats_rc_identity_types"
)

// checkAdmin consumes the single-use human authorization proof bound to one
// operation and returns its claim.
func checkAdmin(value ohrats_rc_identity_types.HumanAuthorization, operation string) (ohrats_rc_identity_admin_consumer.Claim, error) {
	result := ohrats_rc_identity_admin_consumer.Consume(value, operation)
	if result.IsErr() {
		return ohrats_rc_identity_admin_consumer.Claim{}, errors.New(result.Err())
	}
	return result.Ok(), nil
}
