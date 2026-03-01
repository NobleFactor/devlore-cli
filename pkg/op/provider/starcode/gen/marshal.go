// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starcode

import (
	"github.com/NobleFactor/devlore-cli/pkg/op"
	provider "github.com/NobleFactor/devlore-cli/pkg/op/provider/starcode"
)

func init() {
	op.RegisterReceiverParams[provider.Sources]("starcode.sources", SourcesParams)
}
