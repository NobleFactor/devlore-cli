// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

// AllActions returns all actions for registration.
// Both writ and lore use this to ensure the same actions are available.
func AllActions() []Action {
	var actions []Action
	actions = append(actions, FileOps(&FileService{})...)
	actions = append(actions, EncryptionOps(&EncryptionService{})...)
	actions = append(actions, PackageOps(&PackageService{})...)
	actions = append(actions, ShellOps(&ShellService{})...)
	actions = append(actions, ServiceManagerOps(&ServiceManagerService{})...)
	return actions
}
