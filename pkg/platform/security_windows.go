//go:build windows

package platform

import (
	"errors"
	"io/fs"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var ErrUnsafePermissions = errors.New("RC private path has an unsafe Windows ACL or type")

const (
	fileAllAccess = 0x1f01ff
)

// ProtectPrivatePath configures a strict DACL granting only the current user FILE_ALL_ACCESS.
func ProtectPrivatePath(path string, directory bool) error {
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	var inheritance uint32 = windows.NO_INHERITANCE
	if directory {
		inheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}

	ea := windows.EXPLICIT_ACCESS{
		AccessPermissions: fileAllAccess,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValue(uintptr(unsafe.Pointer(sid))),
		},
	}

	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{ea}, nil)
	if err != nil {
		return err
	}

	// Protect DACL and set owner
	secInfo := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, secInfo, nil, nil, acl, nil); err != nil {
		return err
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION, sid, nil, nil, nil); err != nil {
		return err
	}

	return ValidatePrivatePath(path, directory)
}

// ValidatePrivatePath validates that the path has a single explicit DACL entry granting full access to the current user.
func ValidatePrivatePath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return ErrUnsafePermissions
	}
	if directory && !info.IsDir() {
		return ErrUnsafePermissions
	}
	if !directory && !info.Mode().IsRegular() {
		return ErrUnsafePermissions
	}

	userSid, err := currentUserSID()
	if err != nil {
		return err
	}

	secDesc, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}

	owner, _, err := secDesc.Owner()
	if err != nil || owner == nil || !windows.EqualSid(owner, userSid) {
		return ErrUnsafePermissions
	}

	dacl, _, err := secDesc.DACL()
	if err != nil || dacl == nil {
		return ErrUnsafePermissions
	}

	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil || ace == nil {
		return ErrUnsafePermissions
	}

	var expectedInheritance byte = 0
	if directory {
		expectedInheritance = byte(windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT)
	}

	aceSid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if ace.Mask&fileAllAccess != fileAllAccess ||
		ace.Header.AceFlags != expectedInheritance ||
		!windows.EqualSid(aceSid, userSid) {
		return ErrUnsafePermissions
	}

	return nil
}

func currentUserSID() (*windows.SID, error) {
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return nil, err
	}
	defer tok.Close()

	user, err := tok.GetTokenUser()
	if err != nil {
		return nil, err
	}
	return user.User.Sid, nil
}
