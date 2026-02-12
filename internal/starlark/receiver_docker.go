// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// DockerReceiver provides the docker.* Starlark namespace.
//
// Backing implementation: os/exec (exec.Command("docker", ...)).
// Uses kwargs pass-through: any keyword argument is converted to a CLI flag.
//
// Example:
//
//	docker.run("nginx", detach=True, name="web", publish=["80:80"])
//	# Executes: docker run --detach --name web --publish 80:80 nginx
type DockerReceiver struct {
	Receiver
	output io.Writer
}

// NewDockerReceiver creates a new docker receiver.
func NewDockerReceiver(output io.Writer) *DockerReceiver {
	return &DockerReceiver{Receiver: NewReceiver("docker"), output: output}
}

// Attr implements starlark.HasAttrs.
func (d *DockerReceiver) Attr(name string) (starlark.Value, error) {
	switch name {
	case "run":
		return MakeAttr("docker.run", d.run), nil
	case "build":
		return MakeAttr("docker.build", d.build), nil
	case "pull":
		return MakeAttr("docker.pull", d.pull), nil
	case "push":
		return MakeAttr("docker.push", d.push), nil
	case "exec":
		return MakeAttr("docker.exec", d.execCmd), nil
	case "stop":
		return MakeAttr("docker.stop", d.stop), nil
	case "start":
		return MakeAttr("docker.start", d.start), nil
	case "rm":
		return MakeAttr("docker.rm", d.rm), nil
	case "rmi":
		return MakeAttr("docker.rmi", d.rmi), nil
	case "tag":
		return MakeAttr("docker.tag", d.tag), nil
	case "login":
		return MakeAttr("docker.login", d.login), nil
	case "installed":
		return MakeAttr("docker.installed", d.installed), nil
	case "version":
		return MakeAttr("docker.version", d.version), nil
	case "images":
		return MakeAttr("docker.images", d.images), nil
	case "image_exists":
		return MakeAttr("docker.image_exists", d.imageExists), nil
	case "ps":
		return MakeAttr("docker.ps", d.ps), nil
	case "running":
		return MakeAttr("docker.running", d.running), nil
	case "inspect":
		return MakeAttr("docker.inspect", d.inspect), nil
	case "compose_up":
		return MakeAttr("docker.compose_up", d.composeUp), nil
	case "compose_down":
		return MakeAttr("docker.compose_down", d.composeDown), nil
	case "compose_ps":
		return MakeAttr("docker.compose_ps", d.composePs), nil
	default:
		return nil, NoSuchAttrError("docker", name)
	}
}

// AttrNames implements starlark.HasAttrs.
func (d *DockerReceiver) AttrNames() []string {
	return []string{
		"build", "compose_down", "compose_ps", "compose_up", "exec",
		"image_exists", "images", "inspect", "installed", "login",
		"ps", "pull", "push", "rm", "rmi", "run", "running",
		"start", "stop", "tag", "version",
	}
}

// =============================================================================
// Core Operations (kwargs pass-through)
// =============================================================================

func (d *DockerReceiver) run(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("run", args, kwargs)
}

func (d *DockerReceiver) build(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("build", args, kwargs)
}

func (d *DockerReceiver) pull(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("pull", args, kwargs)
}

func (d *DockerReceiver) push(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("push", args, kwargs)
}

func (d *DockerReceiver) execCmd(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("exec", args, kwargs)
}

func (d *DockerReceiver) stop(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("stop", args, kwargs)
}

func (d *DockerReceiver) start(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("start", args, kwargs)
}

func (d *DockerReceiver) rm(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("rm", args, kwargs)
}

func (d *DockerReceiver) rmi(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("rmi", args, kwargs)
}

func (d *DockerReceiver) tag(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("tag", args, kwargs)
}

func (d *DockerReceiver) login(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("login", args, kwargs)
}

// =============================================================================
// Compose Operations (kwargs pass-through)
// =============================================================================

func (d *DockerReceiver) composeUp(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThroughCompose("up", args, kwargs)
}

func (d *DockerReceiver) composeDown(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThroughCompose("down", args, kwargs)
}

func (d *DockerReceiver) composePs(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThroughCompose("ps", args, kwargs)
}

// =============================================================================
// Query Operations
// =============================================================================

func (d *DockerReceiver) installed(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	_, err := exec.LookPath("docker")
	return starlark.Bool(err == nil), nil
}

func (d *DockerReceiver) version(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	output, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("docker", "version", "--format", "{{.Client.Version}}")
		output, err = cmd.Output()
		if err != nil {
			return starlark.String(""), nil
		}
	}
	return starlark.String(strings.TrimSpace(string(output))), nil
}

func (d *DockerReceiver) images(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	cmd := exec.Command("docker", "images", "--format", "{{json .}}")
	output, err := cmd.Output()
	if err != nil {
		return starlark.NewList(nil), nil
	}

	var images []starlark.Value
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		var img map[string]interface{}
		if err := json.Unmarshal([]byte(line), &img); err != nil {
			continue
		}

		dict := starlark.NewDict(4)
		if v, ok := img["ID"]; ok {
			_ = dict.SetKey(starlark.String("id"), starlark.String(fmt.Sprintf("%v", v)))
		}
		if v, ok := img["Repository"]; ok {
			_ = dict.SetKey(starlark.String("repository"), starlark.String(fmt.Sprintf("%v", v)))
		}
		if v, ok := img["Tag"]; ok {
			_ = dict.SetKey(starlark.String("tag"), starlark.String(fmt.Sprintf("%v", v)))
		}
		if v, ok := img["Size"]; ok {
			_ = dict.SetKey(starlark.String("size"), starlark.String(fmt.Sprintf("%v", v)))
		}
		images = append(images, dict)
	}

	return starlark.NewList(images), nil
}

func (d *DockerReceiver) imageExists(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var image string
	if len(args) >= 1 {
		s, _ := starlark.AsString(args[0])
		image = s
	}
	if image == "" {
		return nil, fmt.Errorf("docker.image_exists: image required")
	}

	cmd := exec.Command("docker", "image", "inspect", image)
	err := cmd.Run()
	return starlark.Bool(err == nil), nil
}

func (d *DockerReceiver) ps(_ *starlark.Thread, _ *starlark.Builtin, _ starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdArgs := []string{"ps", "--format", "{{json .}}"}

	for _, kv := range kwargs {
		key := strings.ReplaceAll(string(kv[0].(starlark.String)), "_", "-")
		val := kv[1]
		if b, ok := val.(starlark.Bool); ok && bool(b) {
			cmdArgs = append(cmdArgs, "--"+key)
		} else if s, ok := val.(starlark.String); ok && string(s) != "" {
			cmdArgs = append(cmdArgs, "--"+key, string(s))
		}
	}

	cmd := exec.Command("docker", cmdArgs...)
	output, err := cmd.Output()
	if err != nil {
		return starlark.NewList(nil), nil
	}

	var containers []starlark.Value
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		var ctr map[string]interface{}
		if err := json.Unmarshal([]byte(line), &ctr); err != nil {
			continue
		}

		dict := starlark.NewDict(5)
		if v, ok := ctr["ID"]; ok {
			_ = dict.SetKey(starlark.String("id"), starlark.String(fmt.Sprintf("%v", v)))
		}
		if v, ok := ctr["Names"]; ok {
			_ = dict.SetKey(starlark.String("name"), starlark.String(fmt.Sprintf("%v", v)))
		}
		if v, ok := ctr["Image"]; ok {
			_ = dict.SetKey(starlark.String("image"), starlark.String(fmt.Sprintf("%v", v)))
		}
		if v, ok := ctr["Status"]; ok {
			_ = dict.SetKey(starlark.String("status"), starlark.String(fmt.Sprintf("%v", v)))
		}
		if v, ok := ctr["Ports"]; ok {
			_ = dict.SetKey(starlark.String("ports"), starlark.String(fmt.Sprintf("%v", v)))
		}
		containers = append(containers, dict)
	}

	return starlark.NewList(containers), nil
}

func (d *DockerReceiver) running(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var container string
	if len(args) >= 1 {
		s, _ := starlark.AsString(args[0])
		container = s
	}
	if container == "" {
		return nil, fmt.Errorf("docker.running: container required")
	}

	cmd := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", container)
	output, err := cmd.Output()
	if err != nil {
		return starlark.False, nil
	}

	return starlark.Bool(strings.TrimSpace(string(output)) == "true"), nil
}

func (d *DockerReceiver) inspect(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	var target string
	if len(args) >= 1 {
		s, _ := starlark.AsString(args[0])
		target = s
	}
	if target == "" {
		return nil, fmt.Errorf("docker.inspect: target required")
	}

	cmd := exec.Command("docker", "inspect", target)
	output, err := cmd.Output()
	if err != nil {
		return starlark.String(""), nil
	}

	return starlark.String(strings.TrimSpace(string(output))), nil
}

// =============================================================================
// Kwargs Pass-Through Implementation
// =============================================================================

func (d *DockerReceiver) passThrough(subcommand string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdArgs := []string{subcommand}
	cmdArgs = append(cmdArgs, dockerKwargsToFlags(kwargs)...)

	for _, arg := range args {
		cmdArgs = append(cmdArgs, argToString(arg))
	}

	_, _ = fmt.Fprintf(d.output, "  [docker] %s\n", strings.Join(cmdArgs, " "))

	return d.runDocker(cmdArgs)
}

func (d *DockerReceiver) passThroughCompose(subcommand string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdArgs := []string{"compose", subcommand}
	cmdArgs = append(cmdArgs, dockerKwargsToFlags(kwargs)...)

	for _, arg := range args {
		cmdArgs = append(cmdArgs, argToString(arg))
	}

	_, _ = fmt.Fprintf(d.output, "  [docker] %s\n", strings.Join(cmdArgs, " "))

	return d.runDocker(cmdArgs)
}

// dockerKwargsToFlags converts Starlark kwargs to docker CLI flags.
func dockerKwargsToFlags(kwargs []starlark.Tuple) []string {
	var flags []string

	for _, kv := range kwargs {
		key := strings.ReplaceAll(string(kv[0].(starlark.String)), "_", "-")
		val := kv[1]

		switch v := val.(type) {
		case starlark.Bool:
			if bool(v) {
				flags = append(flags, "--"+key)
			}
		case starlark.String:
			if s := string(v); s != "" {
				flags = append(flags, "--"+key, s)
			}
		case starlark.Int:
			i, _ := v.Int64()
			flags = append(flags, "--"+key, fmt.Sprintf("%d", i))
		case *starlark.List:
			for i := 0; i < v.Len(); i++ {
				flags = append(flags, "--"+key, argToString(v.Index(i)))
			}
		case *starlark.Dict:
			for _, item := range v.Items() {
				k := argToString(item[0])
				val := argToString(item[1])
				flags = append(flags, "--"+key, k+"="+val)
			}
		default:
			flags = append(flags, "--"+key, val.String())
		}
	}

	return flags
}

func (d *DockerReceiver) runDocker(args []string) (starlark.Value, error) {
	cmd := exec.Command("docker", args...)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
	}

	return starlarkstruct.FromStringDict(starlark.String("result"), starlark.StringDict{
		"ok":     starlark.Bool(code == 0),
		"stdout": starlark.String(strings.TrimSpace(stdout.String())),
		"stderr": starlark.String(strings.TrimSpace(stderr.String())),
		"code":   starlark.MakeInt(code),
	}), nil
}
