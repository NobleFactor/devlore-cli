// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package starlark provides the Starlark runtime and host bindings for lore.
package starlark

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/NobleFactor/devlore-cli/internal/host"
)

// Bindings provides lore's host API to Starlark scripts.
type Bindings struct {
	host     host.Host
	features []string
	settings map[string]string
	output   io.Writer
	npm      *NpmBindings
	git      *GitBindings
	docker   *DockerBindings
}

// NewBindings creates a new Bindings instance.
func NewBindings(features []string, settings map[string]string, output io.Writer) *Bindings {
	b := &Bindings{
		host:     host.NewHost(),
		features: features,
		settings: settings,
		output:   output,
	}
	b.npm = NewNpmBindings(b)
	b.git = NewGitBindings(b)
	b.docker = NewDockerBindings(b)
	return b
}

// Globals returns the predeclared globals for Starlark scripts.
func (b *Bindings) Globals() starlark.StringDict {
	return starlark.StringDict{
		"platform": b.platformStruct(),
		"package":  b.packageStruct(),
		"fs":       b.fsStruct(),
		"shell":    b.shellStruct(),
		"http":     b.httpStruct(),
		"archive":  b.archiveStruct(),
		"env":      b.envStruct(),
		"service":  b.serviceStruct(),
		"npm":      b.npm.Struct(),
		"git":      b.git.Struct(),
		"docker":   b.docker.Struct(),
		"log":      b.logStruct(),
		"note":     starlark.NewBuiltin("note", b.noteFunc),
		"warn":     starlark.NewBuiltin("warn", b.warnFunc),
		"error":    starlark.NewBuiltin("error", b.errorFunc),
		"success":  starlark.NewBuiltin("success", b.successFunc),
		"fail":     starlark.NewBuiltin("fail", b.failFunc),
	}
}

// =============================================================================
// Platform
// =============================================================================

func (b *Bindings) platformStruct() *starlarkstruct.Struct {
	p := b.host.Platform()
	return starlarkstruct.FromStringDict(starlark.String("platform"), starlark.StringDict{
		"os":       starlark.String(p.OS),
		"arch":     starlark.String(p.Arch),
		"distro":   starlark.String(p.Distro),
		"version":  starlark.String(p.Version),
		"hostname": starlark.String(p.Hostname),
	})
}

// =============================================================================
// Package
// =============================================================================

func (b *Bindings) packageStruct() *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("package"), starlark.StringDict{
		"manager":   starlark.NewBuiltin("package.manager", b.packageManager),
		"installed": starlark.NewBuiltin("package.installed", b.packageInstalled),
		"version":   starlark.NewBuiltin("package.version", b.packageVersion),
		"install":   starlark.NewBuiltin("package.install", b.packageInstall),
		"remove":    starlark.NewBuiltin("package.remove", b.packageRemove),
		"update":    starlark.NewBuiltin("package.update", b.packageUpdate),
		"feature":   starlark.NewBuiltin("package.feature", b.packageFeature),
		"setting":   starlark.NewBuiltin("package.setting", b.packageSetting),
	})
}

func (b *Bindings) packageManager(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(b.host.PackageManager().Name()), nil
}

func (b *Bindings) packageInstalled(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.Bool(b.host.PackageManager().Installed(name)), nil
}

func (b *Bindings) packageVersion(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.String(b.host.PackageManager().Version(name)), nil
}

func (b *Bindings) packageInstall(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var manager string
	var cask bool
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name, "manager?", &manager, "cask?", &cask); err != nil {
		return nil, err
	}

	fmt.Fprintf(b.output, "  [package] Installing %s", name)
	if manager != "" {
		fmt.Fprintf(b.output, " via %s", manager)
	}
	if cask {
		fmt.Fprintf(b.output, " (cask)")
	}
	fmt.Fprintln(b.output)

	result := b.host.PackageManager().Install(name)
	return b.resultToStarlark(result), nil
}

func (b *Bindings) packageRemove(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}

	fmt.Fprintf(b.output, "  [package] Removing %s\n", name)
	result := b.host.PackageManager().Remove(name)
	return b.resultToStarlark(result), nil
}

func (b *Bindings) packageUpdate(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	fmt.Fprintln(b.output, "  [package] Updating package index")
	result := b.host.PackageManager().Update()
	return b.resultToStarlark(result), nil
}

func (b *Bindings) packageFeature(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}

	for _, f := range b.features {
		if f == name {
			return starlark.True, nil
		}
	}
	return starlark.False, nil
}

func (b *Bindings) packageSetting(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	var defaultValue string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name, "default?", &defaultValue); err != nil {
		return nil, err
	}

	if val, ok := b.settings[name]; ok {
		return starlark.String(val), nil
	}
	return starlark.String(defaultValue), nil
}

// =============================================================================
// Filesystem
// =============================================================================

func (b *Bindings) fsStruct() *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("fs"), starlark.StringDict{
		"exists":   starlark.NewBuiltin("fs.exists", b.fsExists),
		"is_dir":   starlark.NewBuiltin("fs.is_dir", b.fsIsDir),
		"read":     starlark.NewBuiltin("fs.read", b.fsRead),
		"write":    starlark.NewBuiltin("fs.write", b.fsWrite),
		"mkdir":    starlark.NewBuiltin("fs.mkdir", b.fsMkdir),
		"remove":   starlark.NewBuiltin("fs.remove", b.fsRemove),
		"copy":     starlark.NewBuiltin("fs.copy", b.fsCopy),
		"move":     starlark.NewBuiltin("fs.move", b.fsMove),
		"chmod":    starlark.NewBuiltin("fs.chmod", b.fsChmod),
		"symlink":  starlark.NewBuiltin("fs.symlink", b.fsSymlink),
		"which":    starlark.NewBuiltin("fs.which", b.fsWhich),
		"home":     starlark.NewBuiltin("fs.home", b.fsHome),
		"join":     starlark.NewBuiltin("fs.join", b.fsJoin),
		"dirname":  starlark.NewBuiltin("fs.dirname", b.fsDirname),
		"basename": starlark.NewBuiltin("fs.basename", b.fsBasename),
	})
}

func (b *Bindings) fsExists(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	path = b.host.ExpandPath(path)
	_, err := os.Stat(path)
	return starlark.Bool(err == nil), nil
}

func (b *Bindings) fsIsDir(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	path = b.host.ExpandPath(path)
	info, err := os.Stat(path)
	if err != nil {
		return starlark.False, nil
	}
	return starlark.Bool(info.IsDir()), nil
}

func (b *Bindings) fsRead(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	path = b.host.ExpandPath(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(data), nil
}

func (b *Bindings) fsWrite(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, content string
	var mode int = 0o644
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &path, "content", &content, "mode?", &mode); err != nil {
		return nil, err
	}
	path = b.host.ExpandPath(path)

	fmt.Fprintf(b.output, "  [fs] Writing %s\n", path)
	err := os.WriteFile(path, []byte(content), os.FileMode(mode))
	return starlark.Bool(err == nil), nil
}

func (b *Bindings) fsMkdir(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	var parents bool = true
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &path, "parents?", &parents); err != nil {
		return nil, err
	}
	path = b.host.ExpandPath(path)

	var err error
	if parents {
		err = os.MkdirAll(path, 0o755)
	} else {
		err = os.Mkdir(path, 0o755)
	}
	return starlark.Bool(err == nil), nil
}

func (b *Bindings) fsRemove(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	var recursive bool
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &path, "recursive?", &recursive); err != nil {
		return nil, err
	}
	path = b.host.ExpandPath(path)

	var err error
	if recursive {
		err = os.RemoveAll(path)
	} else {
		err = os.Remove(path)
	}
	return starlark.Bool(err == nil), nil
}

func (b *Bindings) fsCopy(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var src, dest string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "src", &src, "dest", &dest); err != nil {
		return nil, err
	}
	src = b.host.ExpandPath(src)
	dest = b.host.ExpandPath(dest)

	input, err := os.ReadFile(src)
	if err != nil {
		return starlark.False, nil
	}
	err = os.WriteFile(dest, input, 0o644)
	return starlark.Bool(err == nil), nil
}

func (b *Bindings) fsMove(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var src, dest string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "src", &src, "dest", &dest); err != nil {
		return nil, err
	}
	src = b.host.ExpandPath(src)
	dest = b.host.ExpandPath(dest)

	err := os.Rename(src, dest)
	return starlark.Bool(err == nil), nil
}

func (b *Bindings) fsChmod(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	var mode int
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &path, "mode", &mode); err != nil {
		return nil, err
	}
	path = b.host.ExpandPath(path)

	err := os.Chmod(path, os.FileMode(mode))
	return starlark.Bool(err == nil), nil
}

func (b *Bindings) fsSymlink(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var src, dest string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "src", &src, "dest", &dest); err != nil {
		return nil, err
	}
	src = b.host.ExpandPath(src)
	dest = b.host.ExpandPath(dest)

	err := os.Symlink(src, dest)
	return starlark.Bool(err == nil), nil
}

func (b *Bindings) fsWhich(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}

	path, err := exec.LookPath(name)
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(path), nil
}

func (b *Bindings) fsHome(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return starlark.String(b.host.HomeDir()), nil
}

func (b *Bindings) fsJoin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	parts := make([]string, len(args))
	for i, arg := range args {
		s, ok := starlark.AsString(arg)
		if !ok {
			return nil, fmt.Errorf("fs.join: argument %d is not a string", i)
		}
		parts[i] = s
	}
	return starlark.String(filepath.Join(parts...)), nil
}

func (b *Bindings) fsDirname(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	return starlark.String(filepath.Dir(path)), nil
}

func (b *Bindings) fsBasename(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	return starlark.String(filepath.Base(path)), nil
}

// =============================================================================
// Shell
// =============================================================================

func (b *Bindings) shellStruct() *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("shell"), starlark.StringDict{
		"exec":  starlark.NewBuiltin("shell.exec", b.shellExec),
		"run":   starlark.NewBuiltin("shell.run", b.shellRun),
		"which": starlark.NewBuiltin("shell.which", b.shellWhich),
	})
}

func (b *Bindings) shellExec(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "command", &command); err != nil {
		return nil, err
	}

	fmt.Fprintf(b.output, "  [shell] %s\n", command)
	result := b.host.RunCommand(command, false)
	return b.resultToStarlark(result), nil
}

func (b *Bindings) shellRun(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var command string
	var shell bool
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "command", &command, "shell?", &shell); err != nil {
		// Try positional only
		if len(args) >= 1 {
			if s, ok := starlark.AsString(args[0]); ok {
				command = s
			}
		}
	}

	fmt.Fprintf(b.output, "  [shell] %s\n", command)
	result := b.host.RunCommand(command, false)
	return b.resultToStarlark(result), nil
}

func (b *Bindings) shellWhich(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		// Try positional
		if len(args) >= 1 {
			if s, ok := starlark.AsString(args[0]); ok {
				name = s
			}
		}
	}

	path, err := exec.LookPath(name)
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(path), nil
}

// =============================================================================
// HTTP
// =============================================================================

func (b *Bindings) httpStruct() *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("http"), starlark.StringDict{
		"download": starlark.NewBuiltin("http.download", b.httpDownload),
		"get":      starlark.NewBuiltin("http.get", b.httpGet),
	})
}

func (b *Bindings) httpDownload(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url, dest string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "url", &url, "dest", &dest); err != nil {
		return nil, err
	}
	dest = b.host.ExpandPath(dest)

	fmt.Fprintf(b.output, "  [http] Downloading %s -> %s\n", url, dest)

	resp, err := http.Get(url)
	if err != nil {
		return b.resultToStarlark(host.Result{OK: false, Stderr: err.Error()}), nil
	}
	defer resp.Body.Close()

	out, err := os.Create(dest)
	if err != nil {
		return b.resultToStarlark(host.Result{OK: false, Stderr: err.Error()}), nil
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return b.resultToStarlark(host.Result{OK: false, Stderr: err.Error()}), nil
	}

	return b.resultToStarlark(host.Result{OK: true}), nil
}

func (b *Bindings) httpGet(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var url string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "url", &url); err != nil {
		return nil, err
	}

	resp, err := http.Get(url)
	if err != nil {
		return starlark.String(""), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return starlark.String(""), nil
	}
	return starlark.String(body), nil
}

// =============================================================================
// Archive
// =============================================================================

func (b *Bindings) archiveStruct() *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("archive"), starlark.StringDict{
		"extract": starlark.NewBuiltin("archive.extract", b.archiveExtract),
	})
}

func (b *Bindings) archiveExtract(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, dest string
	var strip int
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "path", &path, "dest", &dest, "strip?", &strip); err != nil {
		return nil, err
	}
	path = b.host.ExpandPath(path)
	dest = b.host.ExpandPath(dest)

	fmt.Fprintf(b.output, "  [archive] Extracting %s -> %s\n", path, dest)

	// Use tar for extraction
	cmd := fmt.Sprintf("mkdir -p %s && tar -xf %s -C %s", dest, path, dest)
	if strip > 0 {
		cmd = fmt.Sprintf("mkdir -p %s && tar -xf %s -C %s --strip-components=%d", dest, path, dest, strip)
	}

	result := b.host.RunCommand(cmd, false)
	return b.resultToStarlark(result), nil
}

// =============================================================================
// Environment
// =============================================================================

func (b *Bindings) envStruct() *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("env"), starlark.StringDict{
		"get":    starlark.NewBuiltin("env.get", b.envGet),
		"set":    starlark.NewBuiltin("env.set", b.envSet),
		"expand": starlark.NewBuiltin("env.expand", b.envExpand),
	})
}

func (b *Bindings) envGet(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, defaultValue string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name, "default?", &defaultValue); err != nil {
		return nil, err
	}

	value := os.Getenv(name)
	if value == "" {
		value = defaultValue
	}
	return starlark.String(value), nil
}

func (b *Bindings) envSet(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name, value string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name, "value", &value); err != nil {
		return nil, err
	}

	os.Setenv(name, value)
	return starlark.None, nil
}

func (b *Bindings) envExpand(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var template string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "template", &template); err != nil {
		return nil, err
	}

	return starlark.String(os.ExpandEnv(template)), nil
}

// =============================================================================
// Service
// =============================================================================

func (b *Bindings) serviceStruct() *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("service"), starlark.StringDict{
		"exists":  starlark.NewBuiltin("service.exists", b.serviceExists),
		"status":  starlark.NewBuiltin("service.status", b.serviceStatus),
		"start":   starlark.NewBuiltin("service.start", b.serviceStart),
		"stop":    starlark.NewBuiltin("service.stop", b.serviceStop),
		"enable":  starlark.NewBuiltin("service.enable", b.serviceEnable),
		"disable": starlark.NewBuiltin("service.disable", b.serviceDisable),
	})
}

func (b *Bindings) serviceExists(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.Bool(b.host.ServiceManager().Exists(name)), nil
}

func (b *Bindings) serviceStatus(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	return starlark.String(b.host.ServiceManager().Status(name)), nil
}

func (b *Bindings) serviceStart(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	result := b.host.ServiceManager().Start(name)
	return b.resultToStarlark(result), nil
}

func (b *Bindings) serviceStop(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	result := b.host.ServiceManager().Stop(name)
	return b.resultToStarlark(result), nil
}

func (b *Bindings) serviceEnable(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	result := b.host.ServiceManager().Enable(name)
	return b.resultToStarlark(result), nil
}

func (b *Bindings) serviceDisable(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var name string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "name", &name); err != nil {
		return nil, err
	}
	result := b.host.ServiceManager().Disable(name)
	return b.resultToStarlark(result), nil
}

// =============================================================================
// Logging
// =============================================================================

func (b *Bindings) logStruct() *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("log"), starlark.StringDict{
		"info":  starlark.NewBuiltin("log.info", b.noteFunc),
		"warn":  starlark.NewBuiltin("log.warn", b.warnFunc),
		"error": starlark.NewBuiltin("log.error", b.errorFunc),
	})
}

func (b *Bindings) noteFunc(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "msg", &msg); err != nil {
		// Try positional
		if len(args) >= 1 {
			if s, ok := starlark.AsString(args[0]); ok {
				msg = s
			}
		}
	}
	fmt.Fprintf(b.output, "  [note] %s\n", msg)
	return starlark.None, nil
}

func (b *Bindings) warnFunc(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "msg", &msg); err != nil {
		if len(args) >= 1 {
			if s, ok := starlark.AsString(args[0]); ok {
				msg = s
			}
		}
	}
	fmt.Fprintf(b.output, "  [warn] %s\n", msg)
	return starlark.None, nil
}

func (b *Bindings) errorFunc(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if err := starlark.UnpackArgs(fn.Name(), args, kwargs, "msg", &msg); err != nil {
		if len(args) >= 1 {
			if s, ok := starlark.AsString(args[0]); ok {
				msg = s
			}
		}
	}
	fmt.Fprintf(b.output, "  [ERROR] %s\n", msg)
	return nil, fmt.Errorf("phase error: %s", msg)
}

func (b *Bindings) failFunc(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if len(args) >= 1 {
		if s, ok := starlark.AsString(args[0]); ok {
			msg = s
		}
	}
	fmt.Fprintf(b.output, "  [FAIL] %s\n", msg)
	return nil, fmt.Errorf("phase failed: %s", msg)
}

func (b *Bindings) successFunc(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var msg string
	if len(args) >= 1 {
		if s, ok := starlark.AsString(args[0]); ok {
			msg = s
		}
	}
	fmt.Fprintf(b.output, "  [SUCCESS] %s\n", msg)
	return starlark.None, nil
}

// =============================================================================
// Helpers
// =============================================================================

func (b *Bindings) resultToStarlark(r host.Result) *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("result"), starlark.StringDict{
		"ok":         starlark.Bool(r.OK),
		"stdout":     starlark.String(r.Stdout),
		"stderr":     starlark.String(r.Stderr),
		"returncode": starlark.MakeInt(r.Code),
		"code":       starlark.MakeInt(r.Code),
	})
}

// UpdateFeatures allows changing features after creation.
func (b *Bindings) UpdateFeatures(features []string) {
	b.features = features
}

// UpdateSettings allows changing settings after creation.
func (b *Bindings) UpdateSettings(settings map[string]string) {
	b.settings = settings
}
