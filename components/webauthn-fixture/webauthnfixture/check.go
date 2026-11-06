package webauthnfixture

import (
	"bytes"
	"errors"
	"fmt"

	"wit_component/ohrats_rc_webauthn_verifier"
)

// Invoke runs the webauthn-check command.
func Invoke(command string, args []string) (uint32, error) {
	if command != "webauthn-check" || len(args) != 0 {
		return 0, errors.New("usage: rc webauthn-check")
	}
	if err := check(); err != nil {
		return 0, err
	}
	fmt.Println("webauthn verifier: ok")
	return 0, nil
}

func check() error {
	value, err := load()
	if err != nil {
		return err
	}
	registrationRequest, err := value.registrationRequest()
	if err != nil {
		return err
	}
	registration := ohrats_rc_webauthn_verifier.VerifyRegistration(value.Algorithm, registrationRequest)
	if registration.IsErr() {
		return errors.New(registration.Err())
	}
	verified := registration.Ok()
	if verified.Credential.Algorithm != "es256" ||
		verified.Credential.SignCount != 0 ||
		!bytes.Equal(verified.Aaguid, bytes.Repeat([]byte{1}, 16)) ||
		!verified.UserVerified {
		return errors.New("registration verifier returned invalid credential state")
	}

	authenticationRequest, err := value.authenticationRequest(verified.Credential)
	if err != nil {
		return err
	}
	authentication := ohrats_rc_webauthn_verifier.VerifyAuthentication(value.Algorithm, authenticationRequest)
	if authentication.IsErr() {
		return errors.New(authentication.Err())
	}
	verifiedAuthentication := authentication.Ok()
	if verifiedAuthentication.Credential.SignCount != 1 ||
		!verifiedAuthentication.SignCountAdvanced ||
		!verifiedAuthentication.UserVerified {
		return errors.New("authentication verifier returned invalid credential state")
	}

	tampered, err := value.authenticationRequest(verified.Credential)
	if err != nil {
		return err
	}
	if len(tampered.Signature) == 0 {
		return errors.New("fixture signature is empty")
	}
	tampered.Signature = append([]byte{}, tampered.Signature...)
	tampered.Signature[0] ^= 0x01
	if !ohrats_rc_webauthn_verifier.VerifyAuthentication(value.Algorithm, tampered).IsErr() {
		return errors.New("tampered authentication unexpectedly verified")
	}
	return nil
}
