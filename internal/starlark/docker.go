// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starlark

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// DockerBindings provides the docker.* API to Starlark scripts.
//
// Uses kwargs pass-through: any keyword argument is converted to a CLI flag.
// This means all docker flags work automatically without explicit binding code.
//
// Example:
//
//	docker.run("nginx", detach=True, name="web", publish=["80:80"])
//	# Executes: docker run --detach --name web --publish 80:80 nginx
type DockerBindings struct {
	bindings *Bindings
}

// NewDockerBindings creates docker bindings attached to the parent bindings.
func NewDockerBindings(b *Bindings) *DockerBindings {
	return &DockerBindings{bindings: b}
}

// Struct returns the docker.* namespace for Starlark.
func (d *DockerBindings) Struct() *starlarkstruct.Struct {
	return starlarkstruct.FromStringDict(starlark.String("docker"), starlark.StringDict{
		// Core operations (kwargs pass-through)
		"run":   starlark.NewBuiltin("docker.run", d.run),
		"build": starlark.NewBuiltin("docker.build", d.build),
		"pull":  starlark.NewBuiltin("docker.pull", d.pull),
		"push":  starlark.NewBuiltin("docker.push", d.push),
		"exec":  starlark.NewBuiltin("docker.exec", d.execCmd),
		"stop":  starlark.NewBuiltin("docker.stop", d.stop),
		"start": starlark.NewBuiltin("docker.start", d.start),
		"rm":    starlark.NewBuiltin("docker.rm", d.rm),
		"rmi":   starlark.NewBuiltin("docker.rmi", d.rmi),
		"tag":   starlark.NewBuiltin("docker.tag", d.tag),
		"login": starlark.NewBuiltin("docker.login", d.login),

		// Queries (special return types)
		"installed":    starlark.NewBuiltin("docker.installed", d.installed),
		"version":      starlark.NewBuiltin("docker.version", d.version),
		"images":       starlark.NewBuiltin("docker.images", d.images),
		"image_exists": starlark.NewBuiltin("docker.image_exists", d.imageExists),
		"ps":           starlark.NewBuiltin("docker.ps", d.ps),
		"running":      starlark.NewBuiltin("docker.running", d.running),
		"inspect":      starlark.NewBuiltin("docker.inspect", d.inspect),

		// Compose (kwargs pass-through)
		"compose_up":   starlark.NewBuiltin("docker.compose_up", d.composeUp),
		"compose_down": starlark.NewBuiltin("docker.compose_down", d.composeDown),
		"compose_ps":   starlark.NewBuiltin("docker.compose_ps", d.composePs),
	})
}

// =============================================================================
// Core Operations (kwargs pass-through)
// =============================================================================

// run runs a container.
// docker.run("nginx:latest", detach=True, name="web", publish=["80:80", "443:443"])
func (d *DockerBindings) run(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("run", args, kwargs)
}

// build builds an image from a Dockerfile.
// docker.build(".", tag="myapp:latest", file="Dockerfile.prod")
func (d *DockerBindings) build(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("build", args, kwargs)
}

// pull pulls an image from a registry.
// docker.pull("nginx:latest", platform="linux/amd64")
func (d *DockerBindings) pull(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("pull", args, kwargs)
}

// push pushes an image to a registry.
// docker.push("myregistry/myapp:latest")
func (d *DockerBindings) push(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("push", args, kwargs)
}

// execCmd executes a command in a running container.
// docker.exec("container_name", "ls", "-la", interactive=True, tty=True)
func (d *DockerBindings) execCmd(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("exec", args, kwargs)
}

// stop stops a running container.
// docker.stop("container_name", time=10)
func (d *DockerBindings) stop(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("stop", args, kwargs)
}

// start starts a stopped container.
// docker.start("container_name", attach=True)
func (d *DockerBindings) start(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("start", args, kwargs)
}

// rm removes a container.
// docker.rm("container_name", force=True, volumes=True)
func (d *DockerBindings) rm(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("rm", args, kwargs)
}

// rmi removes an image.
// docker.rmi("nginx:latest", force=True)
func (d *DockerBindings) rmi(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("rmi", args, kwargs)
}

// tag tags an image.
// docker.tag("nginx:latest", "myregistry/nginx:v1")
func (d *DockerBindings) tag(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("tag", args, kwargs)
}

// login logs in to a registry.
// docker.login("myregistry.com", username="user", password="pass")
func (d *DockerBindings) login(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThrough("login", args, kwargs)
}

// =============================================================================
// Compose Operations (kwargs pass-through)
// =============================================================================

// composeUp runs docker compose up.
// docker.compose_up(detach=True, build=True, file="docker-compose.prod.yml")
func (d *DockerBindings) composeUp(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThroughCompose("up", args, kwargs)
}

// composeDown runs docker compose down.
// docker.compose_down(volumes=True, rmi="all")
func (d *DockerBindings) composeDown(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThroughCompose("down", args, kwargs)
}

// composePs lists compose containers.
// docker.compose_ps(all=True, format="json")
func (d *DockerBindings) composePs(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return d.passThroughCompose("ps", args, kwargs)
}

// =============================================================================
// Query Operations (special return types)
// =============================================================================

// installed checks if docker is available.
// docker.installed() -> True/False
func (d *DockerBindings) installed(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	_, err := exec.LookPath("docker")
	return starlark.Bool(err == nil), nil
}

// version returns the docker version.
// docker.version() -> "24.0.7"
func (d *DockerBindings) version(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

// images lists local images.
// docker.images() -> [{"id": "abc123", "repository": "nginx", "tag": "latest", "size": "142MB"}, ...]
func (d *DockerBindings) images(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

// imageExists checks if an image exists locally.
// docker.image_exists("nginx:latest") -> True/False
func (d *DockerBindings) imageExists(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

// ps lists containers.
// docker.ps(all=True) -> [{"id": "abc", "name": "web", "image": "nginx", ...}, ...]
func (d *DockerBindings) ps(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdArgs := []string{"ps", "--format", "{{json .}}"}

	// Handle kwargs for ps-specific options
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

// running checks if a container is running.
// docker.running("container_name") -> True/False
func (d *DockerBindings) running(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

// inspect returns detailed info about a container or image.
// docker.inspect("container_name") -> JSON string
func (d *DockerBindings) inspect(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
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

// passThrough converts Starlark args/kwargs to docker CLI arguments.
// Positional args are appended after flags. Kwargs become --key or --key=value.
func (d *DockerBindings) passThrough(subcommand string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdArgs := []string{subcommand}

	// Convert kwargs to flags
	cmdArgs = append(cmdArgs, d.kwargsToFlags(kwargs)...)

	// Append positional args
	for _, arg := range args {
		cmdArgs = append(cmdArgs, argToString(arg))
	}

	_, _ = fmt.Fprintf(d.bindings.output, "  [docker] %s\n", strings.Join(cmdArgs, " "))

	return d.runDocker(cmdArgs)
}

// passThroughCompose handles docker compose commands.
func (d *DockerBindings) passThroughCompose(subcommand string, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	cmdArgs := []string{"compose", subcommand}

	// Convert kwargs to flags
	cmdArgs = append(cmdArgs, d.kwargsToFlags(kwargs)...)

	// Append positional args
	for _, arg := range args {
		cmdArgs = append(cmdArgs, argToString(arg))
	}

	_, _ = fmt.Fprintf(d.bindings.output, "  [docker] %s\n", strings.Join(cmdArgs, " "))

	return d.runDocker(cmdArgs)
}

// kwargsToFlags converts Starlark kwargs to CLI flags.
// Handles: bool (--flag), string (--flag value), list (--flag v1 --flag v2), dict (--flag k=v)
func (d *DockerBindings) kwargsToFlags(kwargs []starlark.Tuple) []string {
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
			// --publish 80:80 --publish 443:443
			for i := 0; i < v.Len(); i++ {
				flags = append(flags, "--"+key, argToString(v.Index(i)))
			}
		case *starlark.Dict:
			// --env FOO=bar --env BAZ=qux
			for _, item := range v.Items() {
				k := argToString(item[0])
				val := argToString(item[1])
				flags = append(flags, "--"+key, k+"="+val)
			}
		default:
			// Fallback: convert to string
			flags = append(flags, "--"+key, val.String())
		}
	}

	return flags
}

// argToString converts a Starlark value to a string for CLI args.
func argToString(val starlark.Value) string {
	switch v := val.(type) {
	case starlark.String:
		return string(v)
	case starlark.Int:
		i, _ := v.Int64()
		return fmt.Sprintf("%d", i)
	case starlark.Bool:
		return fmt.Sprintf("%t", bool(v))
	default:
		return v.String()
	}
}

// runDocker executes docker with the given arguments.
func (d *DockerBindings) runDocker(args []string) (starlark.Value, error) {
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
