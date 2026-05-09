// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package appnet

import (
	"crypto/sha256"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// region EXPORTED METHODS

// region Behaviors

// Addressing reports that appnet.Resource is location-keyed: the canonical URL is the identity. The bytes
// served at that URL are not part of this Resource's identity — that concern belongs to a separate
// stream-shaped Resource (planned: stream.Resource in 13.0(k) sub-step k.10), which Download will
// eventually return instead of bare bytes.
func (r *Resource) Addressing() op.AddressingMode {
	return op.AddressingLocation
}

// Etag returns the canonical URL itself. For a URL-keyed Resource, the URL IS the change-detection token —
// two appnet.Resources with the same URL are the same Resource (same etag); two with different URLs are
// different Resources (different URI, different catalog entry, no shadowing involved). The catalog's Etag
// fast-path therefore always matches for an unchanged appnet.Resource.
//
// Returns:
//   - string: the canonical URL (identical to [op.ResourceBase.URI]).
//   - error: nil under normal conditions.
func (r *Resource) Etag() (string, error) {
	return r.URI(), nil
}

// Digest returns sha256 of the canonical URL. The bytes served at the URL are not part of identity here
// (see [Resource.Addressing]); content addressing of fetched bytes is the future stream.Resource's job.
// Hashing the URL keeps the digest consistent in algorithm with the rest of the system (the catalog's
// [op.ParseDigest] only accepts sha256) and gives appnet.Resource a stable, content-addressable token
// derived from its identity.
//
// Returns:
//   - op.Digest: sha256 algorithm with 32 raw bytes.
//   - error: nil under normal conditions.
func (r *Resource) Digest() (op.Digest, error) {
	h := sha256.Sum256([]byte(r.URI()))
	return op.Digest{Algorithm: "sha256", Bytes: h[:]}, nil
}

// endregion

// endregion