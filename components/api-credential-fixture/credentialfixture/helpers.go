package credentialfixture

import (
	"errors"

	"wit_component/ohrats_rc_api_credentials_types"
)

func valid(value string) error {
	if value == "" || len(value) > 40 {
		return errors.New("invalid fixture")
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if isAlphanumeric(character) || character == '-' {
			continue
		}
		return errors.New("invalid fixture")
	}
	return nil
}

func isAlphanumeric(character byte) bool {
	return (character >= 'a' && character <= 'z') ||
		(character >= 'A' && character <= 'Z') ||
		(character >= '0' && character <= '9')
}

func countKind(values []ohrats_rc_api_credentials_types.Credential, kind ohrats_rc_api_credentials_types.Kind) int {
	count := 0
	for _, value := range values {
		if value.Kind == kind {
			count++
		}
	}
	return count
}

func containsScope(values []ohrats_rc_api_credentials_types.Scope, wanted ohrats_rc_api_credentials_types.Scope) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
