package events

import (
	"encoding/json"
	"errors"

	"wit_component/ohrats_rc_events_types"
)

func (value StoredDetail) MarshalJSON() ([]byte, error) {
	encoded := map[string][]any{}
	switch value.Tag {
	case ohrats_rc_events_types.Detail_Account:
		encoded["Account"] = []any{value.AccountID, value.Name}
	case ohrats_rc_events_types.Detail_Workspace:
		encoded["Workspace"] = []any{value.WorkspaceID, value.Name}
	case ohrats_rc_events_types.Detail_Membership:
		encoded["Membership"] = []any{value.WorkspaceID, value.AccountID, value.Name}
	case ohrats_rc_events_types.Detail_Invitation:
		encoded["Invitation"] = []any{value.WorkspaceID, value.InvitationID}
	case ohrats_rc_events_types.Detail_Device:
		encoded["Device"] = []any{value.WorkspaceID, value.DeviceID, value.Name}
	default:
		return nil, errors.New("invalid stored event detail")
	}
	return encodeJSON(encoded)
}

func (value *StoredDetail) UnmarshalJSON(data []byte) error {
	var encoded map[string][]json.RawMessage
	if err := json.Unmarshal(data, &encoded); err != nil {
		return err
	}
	if len(encoded) != 1 {
		return errors.New("invalid stored event detail")
	}
	for name, fields := range encoded {
		switch name {
		case "Account":
			if len(fields) != 2 {
				return errors.New("invalid stored event detail")
			}
			account, err := decodeString(fields[0])
			if err != nil {
				return err
			}
			displayName, err := decodeName(fields[1])
			if err != nil {
				return err
			}
			*value = StoredDetail{Tag: ohrats_rc_events_types.Detail_Account, AccountID: account, Name: displayName}
		case "Workspace":
			if len(fields) != 2 {
				return errors.New("invalid stored event detail")
			}
			workspace, err := decodeString(fields[0])
			if err != nil {
				return err
			}
			name, err := decodeName(fields[1])
			if err != nil {
				return err
			}
			*value = StoredDetail{Tag: ohrats_rc_events_types.Detail_Workspace, WorkspaceID: workspace, Name: name}
		case "Membership":
			if len(fields) != 3 {
				return errors.New("invalid stored event detail")
			}
			workspace, err := decodeString(fields[0])
			if err != nil {
				return err
			}
			account, err := decodeString(fields[1])
			if err != nil {
				return err
			}
			role, err := decodeName(fields[2])
			if err != nil {
				return err
			}
			*value = StoredDetail{
				Tag:         ohrats_rc_events_types.Detail_Membership,
				WorkspaceID: workspace,
				AccountID:   account,
				Name:        role,
			}
		case "Invitation":
			if len(fields) != 2 {
				return errors.New("invalid stored event detail")
			}
			workspace, err := decodeString(fields[0])
			if err != nil {
				return err
			}
			invitation, err := decodeString(fields[1])
			if err != nil {
				return err
			}
			*value = StoredDetail{
				Tag:          ohrats_rc_events_types.Detail_Invitation,
				WorkspaceID:  workspace,
				InvitationID: invitation,
			}
		case "Device":
			if len(fields) != 3 {
				return errors.New("invalid stored event detail")
			}
			workspace, err := decodeString(fields[0])
			if err != nil {
				return err
			}
			device, err := decodeString(fields[1])
			if err != nil {
				return err
			}
			name, err := decodeName(fields[2])
			if err != nil {
				return err
			}
			*value = StoredDetail{
				Tag:         ohrats_rc_events_types.Detail_Device,
				WorkspaceID: workspace,
				DeviceID:    device,
				Name:        name,
			}
		default:
			return errors.New("invalid stored event detail")
		}
	}
	return nil
}
