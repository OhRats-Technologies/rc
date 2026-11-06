package events

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"wit_component/ohrats_rc_events_types"
)

const (
	maxID          = 128
	maxName        = 120
	maxIdempotency = 128
)

// ValidateRequest enforces bounded, kind/detail-consistent append requests.
func ValidateRequest(value ohrats_rc_events_types.AppendRequest) error {
	if value.OccurredAtMs == 0 {
		return errors.New("event timestamp is required")
	}
	if value.ActorAccountId.IsSome() {
		if err := validateID(value.ActorAccountId.Some(), "actor account id"); err != nil {
			return err
		}
	}
	if value.IdempotencyKey.IsSome() {
		key := value.IdempotencyKey.Some()
		if key == "" || len(key) > maxIdempotency || strings.ContainsFunc(key, unicode.IsControl) {
			return errors.New("invalid idempotency key")
		}
	}
	switch value.Kind {
	case ohrats_rc_events_types.LifecycleKind_AccountCreated,
		ohrats_rc_events_types.LifecycleKind_AccountRenamed,
		ohrats_rc_events_types.LifecycleKind_AccountDeleted:
		if value.Detail.Tag() != ohrats_rc_events_types.Detail_Account {
			return errors.New("event kind does not match structured detail")
		}
		detail := value.Detail.Account()
		if err := validateID(detail.AccountId, "account id"); err != nil {
			return err
		}
		return validateName(pointerFromOption(detail.DisplayName))
	case ohrats_rc_events_types.LifecycleKind_WorkspaceCreated,
		ohrats_rc_events_types.LifecycleKind_WorkspaceRenamed,
		ohrats_rc_events_types.LifecycleKind_WorkspaceDeleted:
		if value.Detail.Tag() != ohrats_rc_events_types.Detail_Workspace {
			return errors.New("event kind does not match structured detail")
		}
		detail := value.Detail.Workspace()
		if err := validateID(detail.WorkspaceId, "workspace id"); err != nil {
			return err
		}
		return validateName(pointerFromOption(detail.Name))
	case ohrats_rc_events_types.LifecycleKind_WorkspaceMemberJoined,
		ohrats_rc_events_types.LifecycleKind_WorkspaceMemberRoleChanged,
		ohrats_rc_events_types.LifecycleKind_WorkspaceMemberLeft:
		if value.Detail.Tag() != ohrats_rc_events_types.Detail_Membership {
			return errors.New("event kind does not match structured detail")
		}
		detail := value.Detail.Membership()
		if err := validateID(detail.WorkspaceId, "workspace id"); err != nil {
			return err
		}
		if err := validateID(detail.AccountId, "account id"); err != nil {
			return err
		}
		return validateName(pointerFromOption(detail.Role))
	case ohrats_rc_events_types.LifecycleKind_WorkspaceInviteCreated,
		ohrats_rc_events_types.LifecycleKind_WorkspaceInviteRevoked:
		if value.Detail.Tag() != ohrats_rc_events_types.Detail_Invitation {
			return errors.New("event kind does not match structured detail")
		}
		detail := value.Detail.Invitation()
		if err := validateID(detail.WorkspaceId, "workspace id"); err != nil {
			return err
		}
		return validateID(detail.InvitationId, "invitation id")
	case ohrats_rc_events_types.LifecycleKind_DeviceEnrolled,
		ohrats_rc_events_types.LifecycleKind_DeviceRenamed,
		ohrats_rc_events_types.LifecycleKind_DeviceOnline,
		ohrats_rc_events_types.LifecycleKind_DeviceOffline,
		ohrats_rc_events_types.LifecycleKind_DeviceRevoked:
		if value.Detail.Tag() != ohrats_rc_events_types.Detail_Device {
			return errors.New("event kind does not match structured detail")
		}
		detail := value.Detail.Device()
		if err := validateID(detail.WorkspaceId, "workspace id"); err != nil {
			return err
		}
		if err := validateID(detail.DeviceId, "device id"); err != nil {
			return err
		}
		return validateName(pointerFromOption(detail.Name))
	default:
		return errors.New("event kind does not match structured detail")
	}
}

func validateID(value string, label string) error {
	if value == "" || len(value) > maxID {
		return fmt.Errorf("invalid %s", label)
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		ascii := character >= '0' && character <= '9' ||
			character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z'
		if !ascii && character != '.' && character != '-' && character != '_' && character != ':' {
			return fmt.Errorf("invalid %s", label)
		}
	}
	return nil
}

func validateName(value *string) error {
	if value == nil {
		return nil
	}
	if strings.TrimSpace(*value) != "" && len(*value) <= maxName && !strings.ContainsFunc(*value, unicode.IsControl) {
		return nil
	}
	return errors.New("invalid bounded event label")
}
