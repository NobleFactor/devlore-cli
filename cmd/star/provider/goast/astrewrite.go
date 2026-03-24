// SPDX-License-Identifier: MIT
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
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Map {
		return schemasFromMap(val)
	}
	// Try as a struct with a ConfigElement that has children.
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
		if rv.Kind() == reflect.Ptr {
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
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	schema := &doctaxonomy.CommentSchema{Name: name}
	if f := rv.FieldByName("Format"); f.IsValid() {
		schema.Format = f.String()
	}
	if f := rv.FieldByName("NodeType"); f.IsValid() {
		schema.NodeType = f.String()
	}
	if f := rv.FieldByName("SummaryPrefix"); f.IsValid() {
		schema.SummaryPrefix = f.String()
	}

	elementsField := rv.FieldByName("Elements")
	if !elementsField.IsValid() || elementsField.Kind() != reflect.Slice {
		return schema
	}

	for i := 0; i < elementsField.Len(); i++ {
		ev := elementsField.Index(i)
		if ev.Kind() == reflect.Interface {
			ev = ev.Elem()
		}
		if ev.Kind() == reflect.Ptr {
			ev = ev.Elem()
		}
		if ev.Kind() != reflect.Struct {
			continue
		}

		se := doctaxonomy.SchemaElement{}
		if f := ev.FieldByName("Name"); f.IsValid() {
			se.Name = f.String()
		}
		if f := ev.FieldByName("Type"); f.IsValid() {
			se.Type = f.String()
		}
		if f := ev.FieldByName("Required"); f.IsValid() {
			se.Required = f.String()
		}
		if f := ev.FieldByName("Cardinality"); f.IsValid() {
			se.Cardinality = f.String()
		}
		if f := ev.FieldByName("Order"); f.IsValid() && f.CanInt() {
			se.Order = int(f.Int())
		}
		if f := ev.FieldByName("Header"); f.IsValid() {
			se.Header = f.String()
		}
		if f := ev.FieldByName("ItemTokens"); f.IsValid() {
			se.ItemTokens = f.String()
		}
		if f := ev.FieldByName("Production"); f.IsValid() {
			se.Production = f.String()
		}
		if f := ev.FieldByName("Consumes"); f.IsValid() {
			se.Consumes = f.String()
		}
		if f := ev.FieldByName("Condition"); f.IsValid() {
			se.Condition = f.String()
		}
		if f := ev.FieldByName("Prefix"); f.IsValid() {
			se.Prefix = f.String()
		}
		if f := ev.FieldByName("Split"); f.IsValid() {
			se.Split = f.String()
		}
		if f := ev.FieldByName("Slots"); f.IsValid() {
			se.Slots = f.String()
		}
		if f := ev.FieldByName("SlotPrefix"); f.IsValid() {
			se.SlotPrefix = f.String()
		}
		schema.Elements = append(schema.Elements, se)
	}

	return schema
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
