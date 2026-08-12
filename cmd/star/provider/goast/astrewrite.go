// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"go/ast"
	"reflect"
	"strings"

	"github.com/NobleFactor/devlore-cli/cmd/star/provider/goast/doctaxonomy"
)

// configNavigator is satisfied by *config.Config without importing the config package — avoids a circular dependency.
type configNavigator interface {
	Navigate(path string) interface{}
}

// schemasFromConfig converts config map data into a SchemaRegistry.
func schemasFromConfig(val interface{}) *doctaxonomy.SchemaRegistry {
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Map {
		return schemasFromMap(val)
	}
	// Try as a struct with a Element that has children.
	if nav, ok := val.(configNavigator); ok {
		_ = nav
	}
	return nil
}

// schemasFromMap converts a map[string]interface{} of schema definitions into a SchemaRegistry.
func schemasFromMap(val interface{}) *doctaxonomy.SchemaRegistry {
	m, ok := val.(map[string]interface{})
	if !ok {
		// Try reflect-based map access for generated config types.
		rv := reflect.ValueOf(val)
		if rv.Kind() == reflect.Pointer {
			rv = rv.Elem()
		}
		if rv.Kind() != reflect.Map {
			return nil
		}
		m = make(map[string]interface{})
		for _, key := range rv.MapKeys() {
			m[key.String()] = rv.MapIndex(key).Interface()
		}
	}

	reg := doctaxonomy.NewSchemaRegistry()
	for name, schemaVal := range m {
		schema := schemaFromConfigVal(name, schemaVal)
		if schema != nil {
			reg.Register(*schema)
		}
	}
	return reg
}

// schemaFromConfigVal converts a single schema config value into a CommentSchema.
func schemaFromConfigVal(name string, val interface{}) *doctaxonomy.CommentSchema {
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	schema := &doctaxonomy.CommentSchema{Name: name}
	reflectStringFields(rv, map[string]*string{
		"Format":        &schema.Format,
		"NodeType":      &schema.NodeType,
		"SummaryPrefix": &schema.SummaryPrefix,
	})

	elementsField := rv.FieldByName("Elements")
	if !elementsField.IsValid() || elementsField.Kind() != reflect.Slice {
		return schema
	}

	for i := 0; i < elementsField.Len(); i++ {
		if se, ok := schemaElementFromValue(elementsField.Index(i)); ok {
			schema.Elements = append(schema.Elements, se)
		}
	}

	return schema
}

// schemaElementFromValue extracts one schema element from a reflected config value, unwrapping
// interface and pointer indirection.
//
// Parameters:
//   - `ev`: the reflected element value.
//
// Returns:
//   - `doctaxonomy.SchemaElement`: the populated element.
//   - `bool`: false when the value does not resolve to a struct.
func schemaElementFromValue(ev reflect.Value) (doctaxonomy.SchemaElement, bool) {

	if ev.Kind() == reflect.Interface {
		ev = ev.Elem()
	}
	if ev.Kind() == reflect.Pointer {
		ev = ev.Elem()
	}
	if ev.Kind() != reflect.Struct {
		return doctaxonomy.SchemaElement{}, false
	}

	se := doctaxonomy.SchemaElement{}
	reflectStringFields(ev, map[string]*string{
		"Name":        &se.Name,
		"Type":        &se.Type,
		"Required":    &se.Required,
		"Cardinality": &se.Cardinality,
		"Header":      &se.Header,
		"ItemTokens":  &se.ItemTokens,
		"Production":  &se.Production,
		"Consumes":    &se.Consumes,
		"Condition":   &se.Condition,
		"Prefix":      &se.Prefix,
		"Split":       &se.Split,
		"Slots":       &se.Slots,
		"SlotPrefix":  &se.SlotPrefix,
	})
	if f := ev.FieldByName("Order"); f.IsValid() && f.CanInt() {
		se.Order = int(f.Int())
	}

	return se, true
}

// reflectStringFields copies the named string fields from a reflected struct into their targets,
// skipping absent fields.
//
// Parameters:
//   - `rv`: the reflected struct value.
//   - `targets`: field name to destination.
func reflectStringFields(rv reflect.Value, targets map[string]*string) {

	for name, target := range targets {
		if f := rv.FieldByName(name); f.IsValid() {
			*target = f.String()
		}
	}
}

// genDeclName returns the primary name for a GenDecl — the first TypeSpec, ValueSpec, or ImportSpec name.
func genDeclName(gd *ast.GenDecl) string {
	for _, spec := range gd.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			return s.Name.Name
		case *ast.ValueSpec:
			if len(s.Names) > 0 {
				return s.Names[0].Name
			}
		}
	}
	return ""
}

// isDelineatorBlock returns true if the raw comment text contains a delineator line (3+ repeated =, -, ~, or *
// characters).
func isDelineatorBlock(raw string) bool {
	for _, line := range strings.Split(raw, "\n") {
		s := strings.TrimSpace(line)
		if len(s) >= 3 {
			first := s[0]
			if first == '=' || first == '-' || first == '~' || first == '*' {
				allSame := true
				for i := 1; i < len(s); i++ {
					if s[i] != first {
						allSame = false
						break
					}
				}
				if allSame {
					return true
				}
			}
		}
	}
	return false
}
