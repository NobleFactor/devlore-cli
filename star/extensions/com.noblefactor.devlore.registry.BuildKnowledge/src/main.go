// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

// Go AST parsing receiver for devlore extensions.
//
// Compiled to WASM (wasm32-wasip1) as a shared memory reactor.
// The host calls alloc to write JSON args into module memory, then calls the
// named function export which returns packed (result_ptr << 32 | result_len).
// The host reads the result and calls dealloc for both pointers.
//
// Build: GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o receivers/go.wasm ./src/
package main

import (
	"encoding/json"
	"unsafe"
)

// PathParams is the common parameter structure: a single path argument.
type PathParams struct {
	Path string `json:"path"`
}

// allocations tracks allocated buffers to prevent GC collection.
// Without this, the GC could collect a buffer while the host is still
// reading from or writing to it.
var allocations = make(map[uintptr][]byte)

// wasmAlloc allocates a byte buffer in the Go heap and returns its address
// in WASM linear memory. The host writes JSON args into this buffer.
//
//go:wasmexport alloc
func wasmAlloc(size int32) int32 {
	buf := make([]byte, size)
	ptr := uintptr(unsafe.Pointer(&buf[0]))
	allocations[ptr] = buf
	return int32(ptr)
}

// wasmDealloc releases a previously allocated buffer, allowing the GC to
// reclaim the memory.
//
//go:wasmexport dealloc
func wasmDealloc(ptr int32, size int32) {
	delete(allocations, uintptr(ptr))
}

// readInput reads bytes from a WASM memory pointer into a Go slice.
func readInput(ptr int32, length int32) []byte {
	if length == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), int(length))
}

// packResult allocates memory for result bytes and returns packed (ptr << 32 | len).
func packResult(data []byte) int64 {
	if len(data) == 0 {
		return 0
	}
	size := int32(len(data))
	resultPtr := wasmAlloc(size)
	dest := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(resultPtr))), int(size))
	copy(dest, data)
	return (int64(resultPtr) << 32) | int64(uint32(size))
}

// unmarshalParams reads JSON params from WASM memory and returns the path.
func unmarshalParams(name string, jsonPtr int32, jsonLen int32) string {
	input := readInput(jsonPtr, jsonLen)
	var params PathParams
	if err := json.Unmarshal(input, &params); err != nil {
		panic(name + ": invalid params: " + err.Error())
	}
	return params.Path
}

// marshalResult marshals a value to JSON and returns a packed pointer.
func marshalResult(name string, result any) int64 {
	output, err := json.Marshal(result)
	if err != nil {
		panic(name + ": marshal result: " + err.Error())
	}
	return packResult(output)
}

//go:wasmexport parse_devlore_api
func wasmParseDevloreAPI(jsonPtr int32, jsonLen int32) int64 {
	path := unmarshalParams("parse_devlore_api", jsonPtr, jsonLen)
	result, err := parseDevloreAPI(path)
	if err != nil {
		panic("parse_devlore_api: " + err.Error())
	}
	return marshalResult("parse_devlore_api", result)
}

//go:wasmexport parse_migrate_knowledge
func wasmParseMigrateKnowledge(jsonPtr int32, jsonLen int32) int64 {
	path := unmarshalParams("parse_migrate_knowledge", jsonPtr, jsonLen)
	result, err := parseMigrateKnowledge(path)
	if err != nil {
		panic("parse_migrate_knowledge: " + err.Error())
	}
	return marshalResult("parse_migrate_knowledge", result)
}

//go:wasmexport parse_execution_ops
func wasmParseExecutionOps(jsonPtr int32, jsonLen int32) int64 {
	path := unmarshalParams("parse_execution_ops", jsonPtr, jsonLen)
	result, err := parseExecutionOps(path)
	if err != nil {
		panic("parse_execution_ops: " + err.Error())
	}
	return marshalResult("parse_execution_ops", result)
}

//go:wasmexport parse_execution_schema
func wasmParseExecutionSchema(jsonPtr int32, jsonLen int32) int64 {
	path := unmarshalParams("parse_execution_schema", jsonPtr, jsonLen)
	result, err := parseExecutionSchema(path)
	if err != nil {
		panic("parse_execution_schema: " + err.Error())
	}
	return marshalResult("parse_execution_schema", result)
}

func main() {} // Required by Go compiler for -buildmode=c-shared. Never called.
