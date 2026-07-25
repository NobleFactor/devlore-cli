// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

import (
	"fmt"
	"path/filepath"
	"sync"

	gosops "github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	"github.com/getsops/sops/v3/cmd/sops/common"
	"github.com/getsops/sops/v3/config"
	"github.com/getsops/sops/v3/stores/dotenv"
	"github.com/getsops/sops/v3/stores/ini"
	"github.com/getsops/sops/v3/stores/json"
	"github.com/getsops/sops/v3/stores/yaml"
	"github.com/getsops/sops/v3/version"
)

// sopsConfigName is the per-directory config file the discovery walk collects.
const sopsConfigName = ".sops.yaml"

// xdgFallbackRelPath is the global fallback config, resolved under $XDG_CONFIG_HOME.
const xdgFallbackRelPath = "devlore/sops.yaml"

// Encrypter encrypts cleartext for the SOPS recipients resolved from the `.sops.yaml` governing a target path.
//
// It owns the per-session config-discovery cache — `visited` memoizes the upward [locate] walk per start
// directory, so a subtree's chain is computed once. One Encrypter is held per encryption provider, which is itself
// per-RuntimeEnvironment; concurrent encryption runs under gather, so the cache is mutex-guarded. getsops does all
// crypto: this type only locates the config and drives the getsops encrypt flow.
type Encrypter struct {
	mutex   sync.Mutex
	visited map[string][]string
}

// NewEncrypter returns an Encrypter with an empty discovery cache.
//
// Returns:
//   - `*Encrypter`: a ready encrypter.
func NewEncrypter() *Encrypter {
	return &Encrypter{visited: make(map[string][]string)}
}

// Encrypt encrypts data for the SOPS recipients governing sourcePath and returns the encrypted SOPS document.
//
// Discovery walks up from sourcePath's directory to rootDir, then the XDG fallback, to locate the `.sops.yaml`
// chain (via [locate], cached). getsops resolves the first config whose creation rule matches sourcePath
// into recipients, and the document is emitted in the format inferred from sourcePath.
//
// Parameters:
//   - `data`: the cleartext to encrypt.
//   - `sourcePath`: the target file's path; selects the creation rule and the document format.
//   - `rootDir`: the upper boundary for the `.sops.yaml` walk (the RuntimeEnvironment Root directory).
//
// Returns:
//   - `[]byte`: the encrypted SOPS document.
//   - `error`: discovery, resolution, or encryption failure.
func (e *Encrypter) Encrypt(data []byte, sourcePath, rootDir string) ([]byte, error) {

	cfg, err := firstMatchingConfig(e.resolve(sourcePath, rootDir), sourcePath)
	if err != nil {
		return nil, err
	}

	store := storeForFormat(detectFormat(sourcePath, data))

	branches, err := store.LoadPlainFile(data)
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: load %s: %w", filepath.Base(sourcePath), err)
	}

	tree := gosops.Tree{
		Branches: branches,
		Metadata: metadataFromConfig(cfg),
		FilePath: sourcePath,
	}

	dataKey, errs := tree.GenerateDataKey()
	if len(errs) > 0 {
		return nil, fmt.Errorf("sops encrypt: generate data key: %v", errs)
	}

	if err := common.EncryptTree(common.EncryptTreeOpts{
		DataKey: dataKey,
		Tree:    &tree,
		Cipher:  aes.NewCipher(),
	}); err != nil {
		return nil, fmt.Errorf("sops encrypt: %w", err)
	}

	ciphertext, err := store.EmitEncryptedFile(tree)
	if err != nil {
		return nil, fmt.Errorf("sops encrypt: emit %s: %w", filepath.Base(sourcePath), err)
	}
	return ciphertext, nil
}

// resolve returns the ordered `.sops.yaml` chain governing sourcePath, memoizing the upward walk per directory.
//
// Parameters:
//   - `sourcePath`: the target file's path.
//   - `rootDir`: the upper boundary for the walk.
//
// Returns:
//   - `[]string`: the ordered config-file chain (deepest in-tree first, XDG fallback last).
func (e *Encrypter) resolve(sourcePath, rootDir string) []string {

	dir := filepath.Dir(sourcePath)

	e.mutex.Lock()
	defer e.mutex.Unlock()

	if chain, ok := e.visited[dir]; ok {
		return chain
	}

	chain := locate(rootDir, dir, sopsConfigName, xdgFallbackRelPath)
	e.visited[dir] = chain
	return chain
}

// firstMatchingConfig returns the first config in chain whose creation rule resolves recipients for sourcePath.
//
// getsops conflates "no matching rule" and parse failures into an error, so a config that does not resolve is
// skipped and the next is tried; only a parse-clean, rule-matching config is returned.
//
// Parameters:
//   - `chain`: the ordered `.sops.yaml` paths, nearest first.
//   - `sourcePath`: the file whose recipients are wanted.
//
// Returns:
//   - `*config.Config`: the resolved config (recipients + encrypt settings).
//   - `error`: when no config in the chain governs sourcePath.
func firstMatchingConfig(chain []string, sourcePath string) (*config.Config, error) {

	for _, confPath := range chain {
		if cfg, err := config.LoadCreationRuleForFile(confPath, sourcePath, nil); err == nil && cfg != nil {
			return cfg, nil
		}
	}
	return nil, fmt.Errorf("sops encrypt: no .sops.yaml creation rule governs %s", sourcePath)
}

// metadataFromConfig maps a resolved getsops [config.Config] onto fresh SOPS document metadata.
//
// Parameters:
//   - `cfg`: the resolved config from [config.LoadCreationRuleForFile].
//
// Returns:
//   - `gosops.Metadata`: metadata carrying the recipients and encrypt settings; the MAC and data key are filled by
//     the encryption.
func metadataFromConfig(cfg *config.Config) gosops.Metadata {
	return gosops.Metadata{
		KeyGroups:               cfg.KeyGroups,
		UnencryptedSuffix:       cfg.UnencryptedSuffix,
		EncryptedSuffix:         cfg.EncryptedSuffix,
		UnencryptedRegex:        cfg.UnencryptedRegex,
		EncryptedRegex:          cfg.EncryptedRegex,
		UnencryptedCommentRegex: cfg.UnencryptedCommentRegex,
		EncryptedCommentRegex:   cfg.EncryptedCommentRegex,
		MACOnlyEncrypted:        cfg.MACOnlyEncrypted,
		Version:                 version.Version,
		ShamirThreshold:         cfg.ShamirThreshold,
	}
}

// storeForFormat returns the getsops store for a detected format name.
//
// Parameters:
//   - `format`: the format name from [detectFormat] (yaml, json, dotenv, ini, binary).
//
// Returns:
//   - `gosops.Store`: the matching store; binary/unknown falls back to the JSON binary store.
func storeForFormat(format string) gosops.Store {
	switch format {
	case "yaml":
		return &yaml.Store{}
	case "json":
		return &json.Store{}
	case "dotenv":
		return &dotenv.Store{}
	case "ini":
		return &ini.Store{}
	default:
		return &json.BinaryStore{}
	}
}
