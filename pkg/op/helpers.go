// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package op

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

// CamelToSnake converts a CamelCase Go identifier to snake_case.
//
// Parameters:
//   - `s`: the CamelCase string (e.g., "WriteText", "ReadBytes").
//
// Returns:
//   - `string`: the snake_case equivalent (e.g., "write_text", "read_bytes").
func CamelToSnake(s string) string {

	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 4)

	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				if unicode.IsLower(prev) || unicode.IsDigit(prev) {
					b.WriteRune('_')
				} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
					b.WriteRune('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// GitStyleChecksum computes a checksum modeled on git's object hashing.
//
// Git hashes objects as HASH("<type> <length>\0<content>"). See [Pro Git, Chapter 10 — Git Objects]. The default hash
// is SHA-1, with SHA-256 opt-in per repository since git 2.29 via `git init --object-format=sha256`. Both variants
// share the same header format and differ only in the hash function.
//
// This function uses SHA-256. When `objectType` is a real git object type (`blob`, `tree`, `commit`, `tag`) and
// `content` is in git's canonical form for that type, output matches `git hash-object` against a SHA-256 repository.
// For custom `objectType` values (e.g., `graph`), output is a stable, content-derived identifier in the git tradition
// without a git-compatible counterpart.
//
// Parameters:
//   - `objectType`: the object type label embedded in the header.
//   - `content`: the content to hash.
//
// Returns:
//   - `string`: the canonical "sha256:<hex>" checksum string.
//
// [Pro Git, Chapter 10 — Git Objects]: https://git-scm.com/book/en/v2/Git-Internals-Git-Objects
func GitStyleChecksum(objectType string, content []byte) string {

	header := fmt.Sprintf("%s %d\x00", objectType, len(content))
	hash := sha256.New()
	hash.Write([]byte(header))
	hash.Write(content)

	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

// parseParameters walks the `announce` map and converts wire-form tokens to fully typed Parameter values.
//
// The wire form arrives at AnnounceProvider, AnnounceResource, and AnnounceType as a map[string][]string of
// codegen-emitted parameter-name tokens. parseParameters is the boundary that converts that wire form into
// runtime-typed Parameter values, so ReceiverType construction and everything below it consume Parameter values
// directly and never see raw tokens. For each method, the function looks up the corresponding `[reflect.Method]` on
// providerType to source per-parameter reflect.Type info, then calls parseParameterToken once per token.
// Cross-parameter validation (variadic-must-be-last, kwargs-must-be-last) is deferred to [NewMethod], which has the
// full Go-method context already.
//
// Parameters:
//   - `providerType`: the announced type's `[reflect.Type]`. Used to resolve per-method reflect.Method values via
//     MethodByName so each parameter token can be paired with its Go parameter type.
//   - `methodParameters`: the codegen-emitted wire map. Each value is a list of parameter-name tokens for one
//     method, in the same order as the Go method's non-receiver parameters (a leading context.Context, if
//     present, is implicit and not represented in the wire list).
//
// Returns:
//   - `map[string][]Parameter`: the parsed map, ready to be passed to NewProviderReceiverType, NewResourceReceiverType,
//     or newReceiverType.
//   - `error`: non-nil if any token is malformed, if a default expression cannot be parsed against its parameter type,
//     if the `announce` map names a method that does not exist on providerType, or if a method's token list has more
//     entries than the Go method has parameter slots.
func parseParameters(providerType reflect.Type, methodParameters map[string][]string) (map[string][]Parameter, error) {

	out := make(map[string][]Parameter, len(methodParameters))

	// Promote value types to pointer types so pointer-receiver methods are visible to MethodByName.
	//
	// Mirrors the promotion in newReceiverType — provider methods are conventionally declared on the pointer receiver,
	// but callers commonly pass the value-type `reflect.Type` to `AnnounceProvider`.

	methodType := providerType

	if methodType.Kind() != reflect.Pointer {
		methodType = reflect.PointerTo(methodType)
	}

	for methodName, tokens := range methodParameters {

		m, ok := methodType.MethodByName(methodName)
		if !ok {
			return nil, fmt.Errorf("method %s: not found on type %s", methodName, providerType)
		}

		ctxOffset := 0

		if m.Type.NumIn() >= 2 && m.Type.In(1) == activationRecordType {
			ctxOffset = 1
		}

		parsed := make([]Parameter, len(tokens))

		for i, token := range tokens {

			paramIndex := 1 + ctxOffset + i
			if paramIndex >= m.Type.NumIn() {
				return nil, fmt.Errorf(
					"method %s: %d parameter tokens but Go method has %d non-receiver param slots",
					methodName, len(tokens), m.Type.NumIn()-1-ctxOffset,
				)
			}

			paramType := m.Type.In(paramIndex)

			p, err := parseParameterToken(token, paramType)
			if err != nil {
				return nil, fmt.Errorf("method %s: param %d (%q): %w", methodName, i, token, err)
			}

			parsed[i] = p
		}

		out[methodName] = parsed
	}

	return out, nil
}
