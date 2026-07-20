// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

// Packer is implemented by content-addressed resources whose bytes travel with a serialized graph document.
//
// A graph must be immutable and portable across machine boundaries. Reference resources ([AddressingLocation]) are
// named by URI in slots and recreate on the target host, but a content resource ([AddressingContent]) IS its bytes —
// a target host cannot reconstruct it from the URI alone, so the bytes cross the boundary in the document's content
// section. Pack produces those bytes; [Unpacker.Unpack] is the inverse. Pack must be deterministic — the same
// resource state yields the same bytes — because the document round-trips through pack → unpack → pack and the
// re-packed section must match what was written.
//
// The invariant: `Addressing() == AddressingContent` ⟹ the type implements [Packer] and [Unpacker]. A
// content-addressable resource that cannot pack its bytes could not cross a machine boundary and could not run
// there — it is an illegal resource, not a degraded one. The resource-enumeration discipline test enforces this.
type Packer interface {

	// Pack returns the resource's transportable content bytes.
	//
	// Returns:
	//   - `[]byte`: the content in the form [Unpacker.Unpack] reconstructs the resource from.
	//   - `error`: non-nil when the content is not materialized (for example a URI-only rehydrated resource) or
	//     cannot be read.
	Pack() ([]byte, error)
}

// Unpacker is the inverse of [Packer]: it reconstructs a content resource from a document's content section.
//
// Unpack is dispatched on a zero value of the resource type — the receiver carries no state; the resource type is
// resolved from the URI fragment's canonical Go type id through the announced inventory
// ([receiverRegistry.UnpackerByTypeID]), so no new registry exists for transport. Implementations MUST verify that
// the reconstructed resource's URI equals `uri`: the URI carries the content digest and is covered by the graph
// checksum and signature, so the equality check is what extends that integrity guarantee over the (unsigned) content
// section — tampered bytes fail here.
type Unpacker interface {

	// Unpack reconstructs the resource from packed content bytes.
	//
	// Parameters:
	//   - `runtimeEnvironment`: the session runtime environment; supplies the local content-addressed store the
	//     bytes materialize into.
	//   - `uri`: the resource's canonical tag URI as recorded in the document; the reconstructed resource must
	//     reproduce it exactly.
	//   - `content`: the packed bytes produced by [Packer.Pack].
	//
	// Returns:
	//   - `Resource`: the reconstructed resource, not interned in any catalog — the caller decides the catalog.
	//   - `error`: non-nil on malformed content, a URI mismatch (integrity failure), or a store write failure.
	Unpack(runtimeEnvironment *RuntimeEnvironment, uri string, content []byte) (Resource, error)
}
