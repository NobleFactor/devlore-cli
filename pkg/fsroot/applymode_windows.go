// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build windows

package fsroot

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// applyMode enforces a restrictive Unix mode as a Windows DACL (issue #405).
//
// Go's [os.Chmod] on Windows toggles only the read-only attribute, so a 0o600 request otherwise
// produces a file every account with directory access can read. This translates the one distinction
// Windows can honor faithfully — private versus not — into an access-control list.
//
// A mode that grants anything to group or other is left alone: the file keeps whatever DACL it
// inherited, which is the correct meaning of "not private". A mode that denies both
// (`perm&0o077 == 0`) gets a **protected** DACL granting the file's own owner, SYSTEM, and
// Administrators, and nothing else.
//
// Two choices deserve stating. The DACL is protected — inheritance is broken — because otherwise
// the inherited ACEs survive and the file stays readable no matter what is granted; breaking
// inheritance is the entire point rather than an optimization. And SYSTEM and Administrators are
// kept because excluding them buys nothing (an administrator takes ownership at will) while
// breaking backup and anti-malware tooling.
//
// Directories additionally propagate the restriction to their children
// ([windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT]); files carry no inheritance flags.
//
// Residual, stated rather than hidden: enforcement is applied by path, not by handle, so a
// sufficiently privileged attacker who can replace the path between the write and this call could
// redirect it. Closing that gap requires handle-based [windows.SetSecurityInfo] at every creation
// site and is tracked with #405 rather than solved here.
//
// Parameters:
//   - `path`: the absolute path just created or modified.
//   - `perm`: the permission bits the caller requested.
//   - `isDir`: whether `path` is a directory, which selects the inheritance flags.
//
// Returns:
//   - `error`: non-nil when the owner cannot be read or the DACL cannot be built or applied. A mode
//     that is not private returns nil without touching the object.
func applyMode(path string, perm os.FileMode, isDir bool) error {

	if !isPrivateMode(perm) {
		return nil
	}

	owner, err := ownerSID(path)
	if err != nil {
		return err
	}

	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return fmt.Errorf("fsroot: create SYSTEM sid: %w", err)
	}

	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return fmt.Errorf("fsroot: create Administrators sid: %w", err)
	}

	inheritance := uint32(windows.NO_INHERITANCE)
	if isDir {
		inheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}

	entries := []windows.EXPLICIT_ACCESS{
		grantFullControl(owner, windows.TRUSTEE_IS_USER, inheritance),
		grantFullControl(system, windows.TRUSTEE_IS_USER, inheritance),
		grantFullControl(administrators, windows.TRUSTEE_IS_GROUP, inheritance),
	}

	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return fmt.Errorf("fsroot: build dacl for %s: %w", path, err)
	}

	// PROTECTED_DACL_SECURITY_INFORMATION is what breaks inheritance. Without it the inherited ACEs
	// remain in force and the object stays accessible regardless of the entries above.
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil)
	if err != nil {
		return fmt.Errorf("fsroot: apply dacl to %s: %w", path, err)
	}

	return nil
}

// region HELPER FUNCTIONS

// grantFullControl builds one allow-everything access entry for `sid`.
//
// Parameters:
//   - `sid`: the trustee to grant.
//   - `trusteeType`: [windows.TRUSTEE_IS_USER] or [windows.TRUSTEE_IS_GROUP].
//   - `inheritance`: the inheritance flags, per object kind.
//
// Returns:
//   - `windows.EXPLICIT_ACCESS`: the constructed entry.
func grantFullControl(
	sid *windows.SID, trusteeType windows.TRUSTEE_TYPE, inheritance uint32,
) windows.EXPLICIT_ACCESS {

	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  trusteeType,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

// ownerSID reads the object's current owner.
//
// The owner is read back rather than assumed to be the calling account: a file created inside a
// directory with an owner-defaulting ACL may belong to someone else, and granting the caller
// instead would lock the real owner out of their own file.
//
// Parameters:
//   - `path`: the object to query.
//
// Returns:
//   - `*windows.SID`: the object's owner.
//   - `error`: non-nil when the security information or its owner cannot be read.
func ownerSID(path string) (*windows.SID, error) {

	descriptor, err := windows.GetNamedSecurityInfo(
		path, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return nil, fmt.Errorf("fsroot: read owner of %s: %w", path, err)
	}

	owner, _, err := descriptor.Owner()
	if err != nil {
		return nil, fmt.Errorf("fsroot: owner sid of %s: %w", path, err)
	}

	return owner, nil
}

// endregion
