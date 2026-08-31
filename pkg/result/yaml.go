// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result

import (
	"encoding/json"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLFormatter renders the value as indented YAML. Two-space indentation; latest stable yaml.v3
// from go-yaml.
type YAMLFormatter struct{}

// Compile-time interface guard.
var _ Formatter = YAMLFormatter{}

// Format encodes value as indented YAML to w.
//
// The yaml.Encoder is closed before return to flush trailing bytes. Close errors are wrapped in the
// returned error chain via the deferred sequence.
func (YAMLFormatter) Format(value any, w io.Writer) (err error) {

	value = yamlNumbers(value)

	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	defer func() {
		if closeErr := enc.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	return enc.Encode(value)
}

// yamlNumbers replaces every [json.Number] with a scalar node carrying its literal digits.
//
// Stage 1 leaves numbers as [json.Number] so a presenter renders the digits it was given rather than a
// float64 rounded past 2^53. [encoding/json] special-cases the type and emits it bare; yaml.v3 does not, and
// [json.Number] is a string type -- so without this, `-o yaml` answered `unit_count: "2"` where `-o json`
// answered `2`. One value, two types, decided by the rendering.
//
// The replacement is a [yaml.Node] rather than a converted int64 or float64 because the point is to keep the
// literal: an integer past 2^53 survives as its digits, and the tag says which kind of number it is.
//
// Parameters:
//   - `value`: the normalized result.
//
// Returns:
//   - `any`: the value with every [json.Number] replaced.
func yamlNumbers(value any) any {

	switch v := value.(type) {

	case json.Number:
		tag := "!!int"
		if strings.ContainsAny(string(v), ".eE") {
			tag = "!!float"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: string(v)}

	case map[string]any:
		replaced := make(map[string]any, len(v))
		for key, child := range v {
			replaced[key] = yamlNumbers(child)
		}
		return replaced

	case []any:
		replaced := make([]any, len(v))
		for i, child := range v {
			replaced[i] = yamlNumbers(child)
		}
		return replaced

	default:
		return value
	}
}
