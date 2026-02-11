# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.
#
# Build rules for com.noblefactor.devlore.registry.BuildKnowledge extension
# Included by top-level Makefile

EXT_BUILD_KNOWLEDGE := com.noblefactor.devlore.registry.BuildKnowledge
EXT_BUILD_KNOWLEDGE_DIR := extensions/$(EXT_BUILD_KNOWLEDGE)
EXT_BUILD_KNOWLEDGE_WASM := $(EXT_BUILD_KNOWLEDGE_DIR)/receivers/go.wasm

# Register this extension
EXTENSIONS += $(EXT_BUILD_KNOWLEDGE)

# Go sources for dependency tracking
EXT_BUILD_KNOWLEDGE_SRCS := $(wildcard $(EXT_BUILD_KNOWLEDGE_DIR)/src/*.go) $(EXT_BUILD_KNOWLEDGE_DIR)/go.mod

# Build WASM receiver (reactor mode)
$(EXT_BUILD_KNOWLEDGE)-build: $(EXT_BUILD_KNOWLEDGE_WASM)

$(EXT_BUILD_KNOWLEDGE_WASM): $(EXT_BUILD_KNOWLEDGE_SRCS)
	@echo "Building: $(EXT_BUILD_KNOWLEDGE)"
	@mkdir -p $(dir $@)
	cd $(EXT_BUILD_KNOWLEDGE_DIR) && GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o receivers/go.wasm ./src/

# Copy to StarlarkAPI extension
$(EXT_BUILD_KNOWLEDGE)-copy: $(EXT_BUILD_KNOWLEDGE_WASM)
	@echo "Copying go.wasm to StarlarkAPI extension"
	cp $(EXT_BUILD_KNOWLEDGE_WASM) extensions/com.noblefactor.devlore.StarlarkAPI/receivers/go.wasm

# Package extension for distribution
$(EXT_BUILD_KNOWLEDGE)-package: $(EXT_BUILD_KNOWLEDGE)-build
	@echo "Packaging: $(EXT_BUILD_KNOWLEDGE)"
	@mkdir -p $(EXT_BUILD_KNOWLEDGE_DIR)/dist
	cd $(EXT_BUILD_KNOWLEDGE_DIR) && zip -r dist/$(EXT_BUILD_KNOWLEDGE).star-ext \
		extension.yaml \
		receivers/*.wasm

# Clean build artifacts
$(EXT_BUILD_KNOWLEDGE)-clean:
	@echo "Cleaning: $(EXT_BUILD_KNOWLEDGE)"
	rm -f $(EXT_BUILD_KNOWLEDGE_WASM)
	rm -f extensions/com.noblefactor.devlore.StarlarkAPI/receivers/go.wasm
	rm -rf $(EXT_BUILD_KNOWLEDGE_DIR)/dist

.PHONY: $(EXT_BUILD_KNOWLEDGE)-build $(EXT_BUILD_KNOWLEDGE)-copy $(EXT_BUILD_KNOWLEDGE)-package $(EXT_BUILD_KNOWLEDGE)-clean
