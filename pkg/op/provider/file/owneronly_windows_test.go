// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

//go:build windows

package file

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// meaningfulAccess is the set of rights that let a trustee reach or reshape a file.
//
// An allow entry granting none of these — SYNCHRONIZE or FILE_READ_ATTRIBUTES alone, say — names a trustee who
// still cannot read the contents, write them, delete the file, or rewrite its access list. Counting such an
// entry as access would report a properly protected file as open.
const meaningfulAccess = windows.FILE_READ_DATA |
	windows.FILE_WRITE_DATA |
	windows.FILE_APPEND_DATA |
	windows.FILE_EXECUTE |
	windows.DELETE |
	windows.WRITE_DAC |
	windows.WRITE_OWNER

// OwnerOnly reports whether the file at `path` grants nothing beyond its owner.
//
// Test-only, and deliberately so: no production path needs to ask this. It exists to verify that enforcement
// happened — that a write which requested owner-only permissions actually produced them — which is the one
// defect a seam assertion cannot catch, since the call can be correct while the effect is absent (#405 phase 2,
// where the unconfined root's WriteFile never applied the mode at all).
//
// Mode bits cannot answer here. [os.FileMode] reports 0666 for every file on Windows whatever its
// access-control list, so the Unix form of this check would pass or fail on something unrelated to who can read
// the file. The list itself has to be read.
//
// Owner-only means both halves of what applyMode writes. The DACL must be **protected** — inheritance broken —
// because inherited entries otherwise survive and keep the file readable whatever is granted explicitly. And
// every entry that actually grants access must name the owner, SYSTEM, or Administrators. Those last two are
// kept because excluding them buys nothing: an administrator takes ownership at will.
//
// Two kinds of entry are passed over. **Deny entries** only ever narrow access, so an unexpected trustee in one
// makes the file more restricted, not less; counting a `DENY Everyone` as a grant would report a hardened file
// as open. And **allow entries conferring no [meaningfulAccess]** name a trustee who cannot reach the contents.
//
// Trustees are compared as SIDs rather than by matching the descriptor's SDDL text. SDDL renders well-known
// accounts as two-letter aliases on some hosts and as numeric SIDs on others, and matching one rendering is
// what made #405 phase 2's first CI run fail against a DACL that was correct.
//
// A file with no DACL is not owner-only: a nil DACL grants everyone full access, the opposite of the intent,
// and must never read as true.
//
// It answers exclusion, never access. A caller asking whether it can open the file should open the file: the
// kernel evaluates the whole token, group memberships included, where reading an access list does not — a
// trustee's rights may arrive through a group whose SID appears nowhere in the entries.
//
// Parameters:
//   - `path`: the absolute path to inspect.
//
// Returns:
//   - `bool`: true when no account beyond the owner, SYSTEM, and Administrators is granted access.
//   - `error`: non-nil when the security descriptor, its control flags, its owner, or an entry cannot be read.
func OwnerOnly(path string) (bool, error) {

	descriptor, err := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return false, fmt.Errorf("owner-only check: read security info for %s: %w", path, err)
	}

	control, _, err := descriptor.Control()
	if err != nil {
		return false, fmt.Errorf("owner-only check: read control flags for %s: %w", path, err)
	}

	if control&windows.SE_DACL_PROTECTED == 0 {
		return false, nil
	}

	dacl, _, err := descriptor.DACL()
	if err != nil {
		return false, fmt.Errorf("owner-only check: read dacl for %s: %w", path, err)
	}

	if dacl == nil {
		return false, nil
	}

	permitted, err := permittedTrustees(descriptor)
	if err != nil {
		return false, err
	}

	return daclGrantsOnly(dacl, permitted, path)
}

// region HELPER FUNCTIONS

// permittedTrustees returns the SIDs an enforced file may grant: its owner, SYSTEM, and Administrators.
//
// Parameters:
//   - `descriptor`: the security descriptor whose owner completes the set.
//
// Returns:
//   - `[]*windows.SID`: the permitted trustees.
//   - `error`: non-nil when the owner or a well-known SID cannot be resolved.
func permittedTrustees(descriptor *windows.SECURITY_DESCRIPTOR) ([]*windows.SID, error) {

	owner, _, err := descriptor.Owner()
	if err != nil {
		return nil, fmt.Errorf("owner-only check: read owner: %w", err)
	}

	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, fmt.Errorf("owner-only check: create SYSTEM sid: %w", err)
	}

	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return nil, fmt.Errorf("owner-only check: create Administrators sid: %w", err)
	}

	return []*windows.SID{owner, system, administrators}, nil
}

// daclGrantsOnly reports whether every access-granting entry in `dacl` names one of `permitted`.
//
// Parameters:
//   - `dacl`: the access-control list to walk.
//   - `permitted`: the trustees an enforced file may grant.
//   - `path`: the inspected path, for the error message.
//
// Returns:
//   - `bool`: true when no granting entry names a trustee outside `permitted`.
//   - `error`: non-nil when an entry cannot be read.
func daclGrantsOnly(dacl *windows.ACL, permitted []*windows.SID, path string) (bool, error) {

	for index := uint32(0); index < uint32(dacl.AceCount); index++ {

		var entry *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &entry); err != nil {
			return false, fmt.Errorf("owner-only check: read dacl entry %d for %s: %w", index, path, err)
		}

		if entry.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}

		if entry.Mask&meaningfulAccess == 0 {
			continue
		}

		//nolint:gosec // G103: the SID follows the ACE header inline; &SidStart is the documented way to reach it.
		trustee := (*windows.SID)(unsafe.Pointer(&entry.SidStart))

		if !isPermittedTrustee(trustee, permitted) {
			return false, nil
		}
	}

	return true, nil
}

// isPermittedTrustee reports whether `trustee` is one of `permitted`.
//
// Parameters:
//   - `trustee`: the SID named by a DACL entry.
//   - `permitted`: the trustees an enforced file may grant.
//
// Returns:
//   - `bool`: true when the trustee is permitted.
func isPermittedTrustee(trustee *windows.SID, permitted []*windows.SID) bool {

	for _, allowed := range permitted {
		if trustee.Equals(allowed) {
			return true
		}
	}

	return false
}

// endregion
