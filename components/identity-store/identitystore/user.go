package identitystore

import (
	"errors"

	witTypes "go.bytecodealliance.org/pkg/wit/types"
	"wit_component/ohrats_rc_identity_types"
)

const userBucket = "users"

type storedUser struct {
	Id          string `json:"id"`
	DisplayName string `json:"display_name"`
	CreatedAtMs uint64 `json:"created_at_ms"`
}

func (value storedUser) view() ohrats_rc_identity_types.User {
	return ohrats_rc_identity_types.User{
		Id:          value.Id,
		DisplayName: value.DisplayName,
		CreatedAtMs: value.CreatedAtMs,
	}
}

func CreateUser(id string, displayName string) (ohrats_rc_identity_types.User, error) {
	user, key, value, err := prepareUser(id, displayName)
	if err != nil {
		return ohrats_rc_identity_types.User{}, err
	}
	if err := insert(userBucket, key, value); err != nil {
		return ohrats_rc_identity_types.User{}, err
	}
	return user, nil
}

func prepareUser(id string, displayName string) (ohrats_rc_identity_types.User, []byte, []byte, error) {
	if err := validID(id, "user id"); err != nil {
		return ohrats_rc_identity_types.User{}, nil, nil, err
	}
	if err := validDisplayName(displayName); err != nil {
		return ohrats_rc_identity_types.User{}, nil, nil, err
	}
	stored := storedUser{Id: id, DisplayName: displayName, CreatedAtMs: nowMs()}
	encoded, err := encode(stored)
	if err != nil {
		return ohrats_rc_identity_types.User{}, nil, nil, err
	}
	return stored.view(), []byte(stored.Id), encoded, nil
}

func GetUser(id string) (witTypes.Option[ohrats_rc_identity_types.User], error) {
	user, found, err := getUser(id)
	if err != nil || !found {
		return witTypes.None[ohrats_rc_identity_types.User](), err
	}
	return witTypes.Some(user), nil
}

func getUser(id string) (ohrats_rc_identity_types.User, bool, error) {
	if err := validID(id, "user id"); err != nil {
		return ohrats_rc_identity_types.User{}, false, err
	}
	value, found, err := getValue(userBucket, []byte(id))
	if err != nil || !found {
		return ohrats_rc_identity_types.User{}, false, err
	}
	stored, err := decode[storedUser](value)
	if err != nil {
		return ohrats_rc_identity_types.User{}, false, err
	}
	return stored.view(), true, nil
}

func AllUsers() ([]ohrats_rc_identity_types.User, error) {
	entries, err := scanAll(userBucket)
	if err != nil {
		return nil, err
	}
	users := make([]ohrats_rc_identity_types.User, 0, len(entries))
	for _, entry := range entries {
		stored, err := decode[storedUser](entry.Value)
		if err != nil {
			return nil, err
		}
		users = append(users, stored.view())
	}
	return users, nil
}

func CountUsers() (uint64, error) {
	entries, err := scanAll(userBucket)
	if err != nil {
		return 0, err
	}
	return uint64(len(entries)), nil
}

func RenameUser(id string, displayName string) (ohrats_rc_identity_types.User, error) {
	if err := validDisplayName(displayName); err != nil {
		return ohrats_rc_identity_types.User{}, err
	}
	current, found, err := getUser(id)
	if err != nil {
		return ohrats_rc_identity_types.User{}, err
	}
	if !found {
		return ohrats_rc_identity_types.User{}, errors.New("user not found")
	}
	current.DisplayName = displayName
	stored := storedUser{
		Id:          current.Id,
		DisplayName: current.DisplayName,
		CreatedAtMs: current.CreatedAtMs,
	}
	encoded, err := encode(stored)
	if err != nil {
		return ohrats_rc_identity_types.User{}, err
	}
	if err := replace(userBucket, []byte(id), encoded); err != nil {
		return ohrats_rc_identity_types.User{}, err
	}
	return current, nil
}
