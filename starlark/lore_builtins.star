# SPDX-License-Identifier: MIT
# Copyright (c) 2025 Noble Factor. All rights reserved.

"""Lore Host Bindings API - Starlark stub for IDE completion.

This file provides type hints and documentation for the Lore host API.
It is not executed; it exists solely for IDE autocompletion support.

To use with JetBrains IDEs (via Bazel or Starlark plugin):
1. Mark this file as a library/stub in your IDE settings
2. IDEs will provide completion for platform.*, package.*, fs.*, etc.

API Namespaces:
    platform.*  - System information (read-only)
    package.*   - Package manager operations
    fs.*        - Filesystem operations
    http.*      - Network operations
    archive.*   - Archive operations
    service.*   - Service management
    env.*       - Environment variables
    lore.*      - Child pipelines and registry access
    shell.*     - Constrained command execution (fallback)
    npm.*       - npm package manager (preferred for Node.js)
    git.*       - Git version control (preferred for git ops)
    docker.*    - Docker containers and images
    json.*      - JSON utilities

Logging Functions:
    note(msg)    - Informational message, continues execution
    warn(msg)    - Warning message, continues execution
    error(msg)   - Fails phase, triggers rollback
    success(msg) - Exits phase successfully

Phase Contract:
    def main(lifecycle, state, features, settings):
        '''
        Args:
            lifecycle: Package metadata from lifecycle.yaml (dict)
            state: Output from previous phase (dict)
            features: Enabled features, e.g. ["pdf-latex"] (list)
            settings: User settings, e.g. {"paper": "letter"} (dict)

        Returns:
            dict: State passed to next phase

        Exits:
            error(msg)   - Fails phase, triggers rollback
            success(msg) - Exits phase successfully (early exit)
            return dict  - Fallthrough success
        '''
        pass
"""

# =============================================================================
# Platform (read-only)
# =============================================================================

platform = struct(
    os = "",         # "darwin", "linux", "windows" (GOOS)
    arch = "",       # "amd64", "arm64" (GOARCH)
    distro = "",     # "ubuntu", "debian", "fedora", "macos", "windows"
    version = "",    # OS version string
    hostname = "",   # Machine hostname
)

# =============================================================================
# Package Manager
# =============================================================================

def _package_manager():
    """Returns the detected package manager name.

    Returns:
        str: "brew", "port", "apt", "dnf", "pacman", "winget", "choco", or "unknown"
    """
    return ""

def _package_installed(name):
    """Check if a package is installed.

    Args:
        name: Package name to check

    Returns:
        bool: True if installed
    """
    return False

def _package_version(name):
    """Get installed version of a package.

    Args:
        name: Package name

    Returns:
        str: Version string, or "" if not installed
    """
    return ""

def _package_install(name = None, names = None):
    """Install package(s) via the native package manager.

    Args:
        name: Single package name (str)
        names: Multiple package names (list of str)

    Returns:
        struct: {ok: bool, error: str}
    """
    return struct(ok = True, error = "", stdout = "", stderr = "")

def _package_remove(name):
    """Remove a package.

    Args:
        name: Package name to remove

    Returns:
        struct: {ok: bool, error: str}
    """
    return struct(ok = True, error = "", stdout = "", stderr = "")

def _package_update():
    """Update package manager index.

    Returns:
        struct: {ok: bool, error: str}
    """
    return struct(ok = True, error = "", stdout = "", stderr = "")

def _package_add_repo(url, key_url = None, name = None):
    """Add a package repository.

    Args:
        url: Repository URL
        key_url: GPG key URL (optional)
        name: Repository name (optional)

    Returns:
        struct: {ok: bool, error: str}
    """
    return struct(ok = True, error = "")

def _package_remove_repo(name):
    """Remove a package repository.

    Args:
        name: Repository name

    Returns:
        struct: {ok: bool, error: str}
    """
    return struct(ok = True, error = "")

package = struct(
    manager = _package_manager,
    installed = _package_installed,
    version = _package_version,
    install = _package_install,
    remove = _package_remove,
    update = _package_update,
    add_repo = _package_add_repo,
    remove_repo = _package_remove_repo,
)

# =============================================================================
# Filesystem
# =============================================================================

def _fs_exists(path):
    """Check if path exists.

    Args:
        path: File or directory path

    Returns:
        bool: True if exists
    """
    return False

def _fs_is_dir(path):
    """Check if path is a directory.

    Args:
        path: Path to check

    Returns:
        bool: True if directory
    """
    return False

def _fs_read(path):
    """Read file contents.

    Args:
        path: File path

    Returns:
        str: File contents, or "" if error
    """
    return ""

def _fs_write(path, content, mode = 0o644):
    """Write content to file.

    Args:
        path: File path
        content: Content to write
        mode: File permissions (default 0o644)

    Returns:
        bool: True if successful
    """
    return True

def _fs_mkdir(path, parents = True):
    """Create directory.

    Args:
        path: Directory path
        parents: Create parent directories (default True)

    Returns:
        bool: True if successful
    """
    return True

def _fs_remove(path):
    """Remove file or directory.

    Args:
        path: Path to remove

    Returns:
        bool: True if successful
    """
    return True

def _fs_copy(src, dest):
    """Copy file.

    Args:
        src: Source path
        dest: Destination path

    Returns:
        bool: True if successful
    """
    return True

def _fs_move(src, dest):
    """Move file.

    Args:
        src: Source path
        dest: Destination path

    Returns:
        bool: True if successful
    """
    return True

def _fs_chmod(path, mode):
    """Change file permissions.

    Args:
        path: File path
        mode: Permissions (e.g., 0o755)

    Returns:
        bool: True if successful
    """
    return True

def _fs_chown(path, user = None, group = None):
    """Change file ownership.

    Args:
        path: File path
        user: Owner username
        group: Group name (optional)

    Returns:
        bool: True if successful
    """
    return True

def _fs_symlink(src, dest):
    """Create symbolic link.

    Args:
        src: Target path (what the link points to)
        dest: Link path (the symlink itself)

    Returns:
        bool: True if successful
    """
    return True

def _fs_glob(pattern):
    """Find files matching pattern.

    Args:
        pattern: Glob pattern (e.g., "/etc/*.conf")

    Returns:
        list: List of matching paths
    """
    return []

def _fs_which(name):
    """Find executable in PATH.

    Args:
        name: Executable name

    Returns:
        str: Full path, or "" if not found
    """
    return ""

def _fs_join(*parts):
    """Join path components.

    Args:
        *parts: Path components

    Returns:
        str: Joined path
    """
    return ""

def _fs_dirname(path):
    """Get directory portion of path.

    Args:
        path: File path

    Returns:
        str: Directory path
    """
    return ""

def _fs_basename(path):
    """Get filename portion of path.

    Args:
        path: File path

    Returns:
        str: Filename (last component)

    Example:
        fs.basename("/opt/app/bin") -> "bin"
    """
    return ""

def _fs_home():
    """Get user home directory.

    Returns:
        str: Home directory path (e.g., "/home/user" or "/Users/user")
    """
    return ""

fs = struct(
    exists = _fs_exists,
    is_dir = _fs_is_dir,
    read = _fs_read,
    write = _fs_write,
    mkdir = _fs_mkdir,
    remove = _fs_remove,
    copy = _fs_copy,
    move = _fs_move,
    chmod = _fs_chmod,
    chown = _fs_chown,
    symlink = _fs_symlink,
    glob = _fs_glob,
    which = _fs_which,
    join = _fs_join,
    dirname = _fs_dirname,
    basename = _fs_basename,
    home = _fs_home,
)

# =============================================================================
# HTTP
# =============================================================================

def _http_download(url, dest, checksum = None, headers = None):
    """Download file from URL.

    Args:
        url: URL to download
        dest: Destination path
        checksum: Optional verification (e.g., "sha256:abc123...")
        headers: Optional request headers dict

    Returns:
        struct: {ok: bool, error: str, path: str}
    """
    return struct(ok = True, error = "", path = "")

def _http_get(url, headers = None):
    """Fetch URL content as string.

    Args:
        url: URL to fetch
        headers: Optional request headers dict

    Returns:
        str: Response body
    """
    return ""

def _http_fetch_json(url, headers = None):
    """Fetch URL and parse as JSON.

    Args:
        url: URL to fetch
        headers: Optional request headers dict

    Returns:
        dict: Parsed JSON response
    """
    return {}

http = struct(
    download = _http_download,
    get = _http_get,
    fetch_json = _http_fetch_json,
)

# =============================================================================
# Archive
# =============================================================================

def _archive_extract(path, dest, strip = 0):
    """Extract archive to destination.

    Supported formats: .tar, .tar.gz, .tar.xz, .tar.bz2, .zip, .7z

    Args:
        path: Archive path
        dest: Destination directory
        strip: Strip leading directory components (default 0)

    Returns:
        struct: {ok: bool, error: str, dest: str}
    """
    return struct(ok = True, error = "", dest = "")

def _archive_list(path):
    """List archive contents.

    Args:
        path: Archive path

    Returns:
        list: List of entry paths in archive
    """
    return []

def _archive_create(path, source, format = "tar.gz"):
    """Create archive from source.

    Args:
        path: Output archive path
        source: Source directory or file
        format: Archive format: "tar", "tar.gz", "tar.xz", "zip" (default "tar.gz")

    Returns:
        struct: {ok: bool, error: str, path: str}
    """
    return struct(ok = True, error = "", path = "")

archive = struct(
    extract = _archive_extract,
    list = _archive_list,
    create = _archive_create,
)

# =============================================================================
# Service Management
# =============================================================================

def _service_enable(name):
    """Enable service to start on boot.

    Args:
        name: Service name

    Returns:
        struct: {ok: bool, error: str}
    """
    return struct(ok = True, error = "")

def _service_disable(name):
    """Disable service from starting on boot.

    Args:
        name: Service name

    Returns:
        struct: {ok: bool, error: str}
    """
    return struct(ok = True, error = "")

def _service_start(name):
    """Start a service.

    Args:
        name: Service name

    Returns:
        struct: {ok: bool, error: str}
    """
    return struct(ok = True, error = "")

def _service_stop(name):
    """Stop a service.

    Args:
        name: Service name

    Returns:
        struct: {ok: bool, error: str}
    """
    return struct(ok = True, error = "")

def _service_status(name):
    """Get service status.

    Args:
        name: Service name

    Returns:
        str: "running", "stopped", "not-found", etc.
    """
    return ""

def _service_exists(name):
    """Check if service exists.

    Args:
        name: Service name

    Returns:
        bool: True if service exists
    """
    return False

def _service_restart(name):
    """Restart a service.

    Args:
        name: Service name

    Returns:
        struct: {ok: bool, error: str}
    """
    return struct(ok = True, error = "")

def _service_install(name, exec_path, user = None, description = None):
    """Install a service unit.

    Platform-specific: creates systemd unit, launchd plist, or Windows service.

    Args:
        name: Service name
        exec_path: Path to executable
        user: User to run as (optional)
        description: Service description (optional)

    Returns:
        struct: {ok: bool, error: str}
    """
    return struct(ok = True, error = "")

def _service_uninstall(name):
    """Remove a service unit.

    Args:
        name: Service name

    Returns:
        struct: {ok: bool, error: str}
    """
    return struct(ok = True, error = "")

service = struct(
    exists = _service_exists,
    status = _service_status,
    start = _service_start,
    stop = _service_stop,
    restart = _service_restart,
    enable = _service_enable,
    disable = _service_disable,
    install = _service_install,
    uninstall = _service_uninstall,
)

# =============================================================================
# Environment Variables
# =============================================================================

def _env_get(name, default = ""):
    """Get environment variable.

    Args:
        name: Variable name
        default: Default value if not set (default "")

    Returns:
        str: Variable value or default
    """
    return ""

def _env_set(name, value):
    """Set environment variable for current process.

    Args:
        name: Variable name
        value: Variable value
    """
    pass

def _env_expand(template):
    """Expand environment variables in template.

    Args:
        template: String with ${VAR} or $VAR placeholders

    Returns:
        str: Expanded string

    Example:
        env.expand("${HOME}/.config") -> "/home/user/.config"
    """
    return ""

def _env_path_add(dir):
    """Add directory to PATH.

    Args:
        dir: Directory to add
    """
    pass

def _env_path_remove(dir):
    """Remove directory from PATH.

    Args:
        dir: Directory to remove
    """
    pass

env = struct(
    get = _env_get,
    set = _env_set,
    expand = _env_expand,
    path_add = _env_path_add,
    path_remove = _env_path_remove,
)

# =============================================================================
# Child Pipelines and Registry
# =============================================================================

def _lore_deploy(package, features = None, settings = None):
    """Deploy another package via child pipeline.

    This enables packages to install their dependencies using lore itself.
    For example, pandoc --with pdf-latex can deploy basictex.

    Args:
        package: Package name to deploy
        features: List of features to enable (optional)
        settings: Dict of settings (optional)

    Returns:
        struct: {ok: bool, package: str, error: str, message: str}

    Example:
        lore.deploy(
            package="basictex",
            features=["pdf"],
            settings={"paper": "letter"}
        )
    """
    return struct(ok = True, package = "", error = "", message = "")

def _lore_deploy_many(packages):
    """Deploy multiple packages via child pipelines.

    Args:
        packages: List of dicts, each with "package", "features" (optional), "settings" (optional)

    Returns:
        struct: {ok: bool, results: list, error: str}

    Example:
        lore.deploy_many([
            {"package": "nodejs", "features": ["lts"]},
            {"package": "yarn"}
        ])
    """
    return struct(ok = True, results = [], error = "")

def _lore_registry_lookup(package):
    """Query registry for package lifecycle manifest.

    Args:
        package: Package name

    Returns:
        dict: Lifecycle manifest data, or None if not found
    """
    return None

def _lore_registry_search(query):
    """Search registry for packages.

    Args:
        query: Search query string

    Returns:
        list: List of matching package names
    """
    return []

def _lore_current_package():
    """Get current package name being processed.

    Returns:
        str: Package name
    """
    return ""

def _lore_current_phase():
    """Get current phase being executed.

    Returns:
        str: Phase name ("prepare", "install", "provision", "verify")
    """
    return ""

def _lore_receipt_path():
    """Get path to receipt being built.

    Returns:
        str: Receipt file path
    """
    return ""

lore = struct(
    deploy = _lore_deploy,
    deploy_many = _lore_deploy_many,
    registry_lookup = _lore_registry_lookup,
    registry_search = _lore_registry_search,
    current_package = _lore_current_package,
    current_phase = _lore_current_phase,
    receipt_path = _lore_receipt_path,
)

# =============================================================================
# Constrained Shell Execution
# =============================================================================

def _shell_exec(command, allowed_commands = None, env = None):
    """Execute command with constraints.

    This is an escape hatch for commands not covered by the structured API.
    Use sparingly; prefer structured operations when possible.

    Args:
        command: Command string to execute
        allowed_commands: Whitelist of allowed command names (optional)
        env: Environment variables dict (optional)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}

    Example:
        shell.exec(
            command="tlmgr install bookmark",
            allowed_commands=["tlmgr"],
            env={"PATH": "/usr/local/texlive/2024/bin/x86_64-linux"}
        )
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

shell = struct(
    exec = _shell_exec,
)

# =============================================================================
# JSON
# =============================================================================

def _json_parse(s):
    """Parse JSON string.

    Args:
        s: JSON string

    Returns:
        Parsed value (dict, list, str, int, float, bool, or None)
    """
    return None

def _json_encode(value):
    """Encode value to JSON string.

    Args:
        value: Value to encode

    Returns:
        str: JSON string
    """
    return ""

json = struct(
    parse = _json_parse,
    encode = _json_encode,
)

# =============================================================================
# npm Package Manager (High-Value Runtime Binding)
# =============================================================================

def _npm_install(*packages, global = False, save = True, save_dev = False):
    """Install npm packages.

    Args:
        *packages: Package names to install (or none for package.json)
        global: Install globally (-g)
        save: Save to dependencies (default True)
        save_dev: Save to devDependencies

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}

    Example:
        npm.install("astro", global=True)
        npm.install("react", "react-dom", save=True)
        npm.install()  # Install from package.json
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _npm_uninstall(*packages, global = False):
    """Uninstall npm packages.

    Args:
        *packages: Package names to uninstall
        global: Uninstall globally

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _npm_installed(name, global = True):
    """Check if a package is installed.

    Args:
        name: Package name
        global: Check global packages (default True)

    Returns:
        bool: True if installed
    """
    return False

def _npm_version(name, global = True):
    """Get installed version of a package.

    Args:
        name: Package name
        global: Check global packages (default True)

    Returns:
        str: Version string, or "" if not installed
    """
    return ""

def _npm_list_global():
    """List globally installed packages.

    Returns:
        list: Package names
    """
    return []

def _npm_run(script):
    """Run an npm script from package.json.

    Args:
        script: Script name (e.g., "build", "dev")

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _npm_exec(*args):
    """Execute a package binary via npx.

    Args:
        *args: Command and arguments

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}

    Example:
        npm.exec("create-astro@latest")
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _npm_init(yes = False):
    """Initialize a new package.json.

    Args:
        yes: Accept defaults (--yes)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _npm_prefix():
    """Get the global prefix path.

    Returns:
        str: Global prefix (e.g., "/usr/local" or "~/.npm-global")
    """
    return ""

npm = struct(
    install = _npm_install,
    uninstall = _npm_uninstall,
    installed = _npm_installed,
    version = _npm_version,
    list_global = _npm_list_global,
    run = _npm_run,
    exec = _npm_exec,
    init = _npm_init,
    prefix = _npm_prefix,
)

# =============================================================================
# Git (High-Value Runtime Binding)
# =============================================================================

def _git_clone(url, dest = None, branch = None, depth = 0):
    """Clone a repository.

    Args:
        url: Repository URL
        dest: Destination directory (optional)
        branch: Branch to clone (optional)
        depth: Shallow clone depth, 0 for full (optional)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}

    Example:
        git.clone("https://github.com/user/repo", dest="/tmp/repo", depth=1)
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _git_pull(rebase = False):
    """Pull from remote.

    Args:
        rebase: Use --rebase (default False)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _git_checkout(ref, create = False):
    """Checkout a branch or tag.

    Args:
        ref: Branch, tag, or commit to checkout
        create: Create branch with -b (default False)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _git_version():
    """Get git version.

    Returns:
        str: Version string (e.g., "2.43.0")
    """
    return ""

def _git_installed():
    """Check if git is available.

    Returns:
        bool: True if git is installed
    """
    return False

def _git_repo_root():
    """Get repository root directory.

    Returns:
        str: Repository root path, or "" if not in a repo
    """
    return ""

def _git_current_branch():
    """Get current branch name.

    Returns:
        str: Branch name (e.g., "main")
    """
    return ""

def _git_remote_url(remote = "origin"):
    """Get URL of a remote.

    Args:
        remote: Remote name (default "origin")

    Returns:
        str: Remote URL
    """
    return ""

def _git_is_clean():
    """Check if working directory is clean.

    Returns:
        bool: True if no uncommitted changes
    """
    return False

def _git_latest_tag():
    """Get most recent tag.

    Returns:
        str: Tag name (e.g., "v1.2.3"), or "" if no tags
    """
    return ""

def _git_commit_hash(short = False):
    """Get current commit hash.

    Args:
        short: Return short hash (default False)

    Returns:
        str: Commit hash
    """
    return ""

def _git_config_get(key, global_ = False):
    """Get a git config value.

    Args:
        key: Config key (e.g., "user.email")
        global_: Use --global (default False)

    Returns:
        str: Config value, or "" if not set
    """
    return ""

def _git_config_set(key, value, global_ = False):
    """Set a git config value.

    Args:
        key: Config key (e.g., "user.email")
        value: Config value
        global_: Use --global (default False)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

git = struct(
    clone = _git_clone,
    pull = _git_pull,
    checkout = _git_checkout,
    version = _git_version,
    installed = _git_installed,
    repo_root = _git_repo_root,
    current_branch = _git_current_branch,
    remote_url = _git_remote_url,
    is_clean = _git_is_clean,
    latest_tag = _git_latest_tag,
    commit_hash = _git_commit_hash,
    config_get = _git_config_get,
    config_set = _git_config_set,
)

# =============================================================================
# Docker (High-Value Runtime Binding)
# =============================================================================

def _docker_pull(image, tag = "latest"):
    """Pull an image from registry.

    Args:
        image: Image name (e.g., "nginx", "myregistry.io/app")
        tag: Image tag (default "latest")

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _docker_run(image, command = None, name = None, detach = False, rm = False,
                ports = None, volumes = None, env = None, network = None,
                user = None, workdir = None):
    """Run a container.

    Args:
        image: Image name
        command: Command to run (optional)
        name: Container name (optional)
        detach: Run in background (-d)
        rm: Remove after exit (--rm)
        ports: Port mappings, e.g., {"8080": "80"} or ["8080:80"]
        volumes: Volume mounts, e.g., {"/host": "/container"} or ["/host:/container"]
        env: Environment variables dict
        network: Network name
        user: User to run as
        workdir: Working directory in container

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}

    Example:
        docker.run("nginx", detach=True, ports={"8080": "80"})
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _docker_build(path = ".", tag = None, dockerfile = None, args = None, no_cache = False):
    """Build an image.

    Args:
        path: Build context path (default ".")
        tag: Image tag
        dockerfile: Dockerfile path (optional)
        args: Build arguments dict
        no_cache: Disable cache (default False)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _docker_push(image, tag = None):
    """Push an image to registry.

    Args:
        image: Image name
        tag: Image tag (optional)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _docker_exec(container, command, user = None, workdir = None):
    """Execute command in running container.

    Args:
        container: Container name or ID
        command: Command to execute
        user: User to run as (optional)
        workdir: Working directory (optional)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _docker_stop(container, time = 10):
    """Stop a running container.

    Args:
        container: Container name or ID
        time: Seconds to wait before killing (default 10)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _docker_rm(container, force = False, volumes = False):
    """Remove a container.

    Args:
        container: Container name or ID
        force: Force remove running container (default False)
        volumes: Remove associated volumes (default False)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _docker_rmi(image, force = False):
    """Remove an image.

    Args:
        image: Image name or ID
        force: Force remove (default False)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _docker_installed():
    """Check if Docker is available.

    Returns:
        bool: True if docker command exists
    """
    return False

def _docker_version():
    """Get Docker version.

    Returns:
        str: Version string (e.g., "24.0.7")
    """
    return ""

def _docker_images(name = None):
    """List images.

    Args:
        name: Filter by image name (optional)

    Returns:
        list: List of dicts with {repository, tag, id, size, created}
    """
    return []

def _docker_image_exists(image, tag = "latest"):
    """Check if image exists locally.

    Args:
        image: Image name
        tag: Image tag (default "latest")

    Returns:
        bool: True if image exists locally
    """
    return False

def _docker_ps(all = False):
    """List containers.

    Args:
        all: Include stopped containers (default False)

    Returns:
        list: List of dicts with {id, image, command, created, status, ports, names}
    """
    return []

def _docker_running(container):
    """Check if container is running.

    Args:
        container: Container name or ID

    Returns:
        bool: True if running
    """
    return False

def _docker_inspect(target):
    """Inspect container or image.

    Args:
        target: Container or image name/ID

    Returns:
        dict: Inspection data, or {} on error
    """
    return {}

def _docker_compose_up(file = None, detach = True, build = False, services = None):
    """Start services with docker compose.

    Args:
        file: Compose file path (optional, default docker-compose.yml)
        detach: Run in background (default True)
        build: Build images before starting (default False)
        services: List of specific services to start (optional)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

def _docker_compose_down(file = None, volumes = False, remove_orphans = False):
    """Stop and remove compose services.

    Args:
        file: Compose file path (optional)
        volumes: Remove named volumes (default False)
        remove_orphans: Remove orphan containers (default False)

    Returns:
        struct: {ok: bool, stdout: str, stderr: str, code: int}
    """
    return struct(ok = True, stdout = "", stderr = "", code = 0)

docker = struct(
    pull = _docker_pull,
    run = _docker_run,
    build = _docker_build,
    push = _docker_push,
    exec = _docker_exec,
    stop = _docker_stop,
    rm = _docker_rm,
    rmi = _docker_rmi,
    installed = _docker_installed,
    version = _docker_version,
    images = _docker_images,
    image_exists = _docker_image_exists,
    ps = _docker_ps,
    running = _docker_running,
    inspect = _docker_inspect,
    compose_up = _docker_compose_up,
    compose_down = _docker_compose_down,
)

# =============================================================================
# Logging Functions
# =============================================================================

def note(message):
    """Log informational message. Continues execution.

    Args:
        message: Message to log
    """
    pass

def warn(message):
    """Log warning message. Continues execution.

    Args:
        message: Message to log
    """
    pass

def error(message):
    """Log error and fail phase. Triggers rollback.

    Args:
        message: Error message

    Raises:
        PhaseError: Always raises, never returns
    """
    pass

def success(message, state = None):
    """Log success and exit phase early.

    Args:
        message: Success message
        state: Optional state dict to pass to next phase

    Raises:
        PhaseSuccess: Always raises, never returns
    """
    pass

