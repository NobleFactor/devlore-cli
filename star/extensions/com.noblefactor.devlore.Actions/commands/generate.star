# SPDX-License-Identifier: MIT
# Copyright Noble Factor. All rights reserved.

# generate.star - Generate receivers and actions from provider structs
#
# Reads a provider struct's methods via goast.methods(), then calls
# goast.render() to produce planned receivers, graph actions, and
# immediate receivers.
#
# The Provider struct carries directives:
#
# // +devlore:access= controls which artifacts ALL its methods appear in:
#   access=immediate  — immediate receiver only (default if no directive)
#   access=planned    — planned receiver + graph action wrapper
#   access=both       — all three artifacts
#
# // +devlore:lifetime= declares provider lifecycle semantics:
#   lifetime=stateless — safe to cache indefinitely (default if no directive)
#   lifetime=phase     — fresh instance per phase; cleanup between phases
#   lifetime=session   — single instance for session; cleanup at session end
#
# Methods carry directives:
#
# // +devlore:defaults param=value,... marks params as optional with defaults
# // +devlore:struct_param var=Type expands a struct param to individual kwargs
#
# Generated files live in gen/ subpackage with provider import alias.

# Infrastructure methods excluded from code generation -- not starlark-facing.
SKIP_METHODS = [
    "Attr",
    "AttrNames",
    "Freeze",
    "Hash",
    "ResolveAttr",
    "String",
    "Truth",
    "Type",
]

# Template to output filename mapping.
GEN_TEMPLATE_FILES = {
    "provider": "gen/provider.gen.go",
    "receiver_type_test": "gen/receiver_type.gen_test.go",
    "module_test": "gen/module.gen_test.go",
    "action_test": "gen/action.gen_test.go",
    "node_builder_test": "gen/node_builder.gen_test.go",
    "resource": "gen/resource.gen.go",
    # dependent_type uses dynamic filenames: gen/<type_snake>.gen.go
}

# Local templates shipped with this extension (loaded from templates/ dir).
LOCAL_TEMPLATES = {
    "provider": "provider.gen.go.template",
    "receiver_type_test": "receiver_type.gen_test.go.template",
    "module_test": "module.gen_test.go.template",
    "action_test": "action.gen_test.go.template",
    "node_builder_test": "node_builder.gen_test.go.template",
    "resource": "resource.gen.go.template",
    "dependent_type": "dependent_type.gen.go.template",
}

# Primitive Go types — return types NOT in this set are considered custom.
PRIMITIVE_RETURNS = [
    "string", "bool", "int", "int64", "[]byte", "[]string",
    "error", "(error)",
    "(string, error)", "(bool, error)", "(int, error)", "(int64, error)",
    "([]byte, error)", "([]string, error)",
]

def load_template(name, ext_dir):
    """Load template content by name from the extension's templates/ directory."""
    if name not in LOCAL_TEMPLATES:
        fail("unknown template: " + name)
    path = file.join(ext_dir, "templates", LOCAL_TEMPLATES[name])
    return file.read_text(path)

def to_snake(name):
    """Convert CamelCase to snake_case."""
    result = []
    for i in range(len(name)):
        ch = name[i]
        if ch.isupper():
            if i > 0:
                prev = name[i - 1]
                if prev.islower():
                    result.append("_")
                elif prev.isupper() and i + 1 < len(name) and name[i + 1].islower():
                    result.append("_")
            result.append(ch.lower())
        else:
            result.append(ch)
    return "".join(result)

def access_title(access):
    """Convert an access string to its Go title-case constant suffix.

    'immediate' → 'Immediate', 'planned' → 'Planned', 'both' → 'Both'
    """
    return access[0].upper() + access[1:]

def lifetime_title(lifetime):
    """Convert a lifetime string to its Go title-case constant suffix.

    'stateless' → 'Stateless', 'phase' → 'Phase', 'session' → 'Session'
    """
    return lifetime[0].upper() + lifetime[1:]

def lc_first(name):
    """Lowercase the first character of a name."""
    if not name:
        return name
    return name[0].lower() + name[1:]

# =============================================================================
# Directive Parsing
# =============================================================================

def struct_access(path):
    """Extract the +devlore:access level from the Provider struct's doc comment.

    Returns 'immediate' if no directive is found (the default).
    """
    doc = goast.type_doc(path)
    for line in doc.split("\n"):
        line = line.strip().lstrip("/").strip()
        if "+devlore:access=" in line:
            idx = line.index("+devlore:access=")
            value = line[idx + len("+devlore:access="):].strip()
            if value not in ["immediate", "planned", "both"]:
                fail("invalid +devlore:access value %r on Provider struct (valid: immediate, planned, both)" % value)
            return value
    return "immediate"

def struct_lifetime(path):
    """Extract the +devlore:lifetime level from the Provider struct's doc comment.

    Returns 'stateless' if no directive is found (the default).
    """
    doc = goast.type_doc(path)
    for line in doc.split("\n"):
        line = line.strip().lstrip("/").strip()
        if "+devlore:lifetime=" in line:
            idx = line.index("+devlore:lifetime=")
            value = line[idx + len("+devlore:lifetime="):].strip()
            if value not in ["stateless", "phase", "session"]:
                fail("invalid +devlore:lifetime value %r on Provider struct (valid: stateless, phase, session)" % value)
            return value
    return "stateless"

def struct_root(path):
    """Extract the +devlore:root flag from the Provider struct's doc comment.

    The +devlore:root=true directive sets the RoleRoot placement-zone bit on
    the generated AnnounceProvider call, causing the provider's methods to
    surface flat at their access-defined namespace root rather than nested
    under the provider's own name. See Phase 8 D12 for the semantics.

    Returns False if no directive is found (the default — methods surface
    nested under the provider's name).
    """
    doc = goast.type_doc(path)
    for line in doc.split("\n"):
        line = line.strip().lstrip("/").strip()
        if "+devlore:root=" in line:
            idx = line.index("+devlore:root=")
            value = line[idx + len("+devlore:root="):].strip()
            if value not in ["true", "false"]:
                fail("invalid +devlore:root value %r on Provider struct (valid: true, false)" % value)
            return value == "true"
    return False

def parse_defaults(doc, method_name):
    """Parse +devlore:defaults from a method doc comment.

    Returns a dict of param_name → default_value_string, or empty dict.
    Example: '+devlore:defaults gitignore=true,includeBzl=true'
    → {"gitignore": "true", "includeBzl": "true"}

    Syntactic validation (each violation aborts codegen via fail):
      - every pair must contain '='
      - the parameter name (left side) must be non-empty
      - no parameter name may appear more than once

    An empty value (e.g., 'name=') is permitted — it marks the parameter as
    optional with no concrete default, equivalent to writing 'name?' in the
    wire token. compute_param_names_list collapses this case.

    Semantic validation (cross-checked against the method's parameter list)
    happens in build_method_descriptors after the params dict is built.
    """
    result = {}
    for line in doc.split("\n"):
        line = line.strip().lstrip("/").strip()
        if "+devlore:defaults " in line:
            idx = line.index("+devlore:defaults ")
            pairs = line[idx + len("+devlore:defaults "):].strip()
            for pair in pairs.split(","):
                pair = pair.strip()
                if "=" not in pair:
                    fail("method %s: +devlore:defaults pair %r missing '='" % (method_name, pair))
                k, v = pair.split("=", 1)
                k = k.strip()
                v = v.strip()
                if k == "":
                    fail("method %s: +devlore:defaults pair %r has empty parameter name" % (method_name, pair))
                if k in result:
                    fail("method %s: +devlore:defaults specifies %r more than once" % (method_name, k))
                result[k] = v
    return result

def parse_struct_param(doc):
    """Parse +devlore:struct_param from a method doc comment.

    Returns a dict of var_name → struct_type_name, or empty dict.
    Example: '+devlore:struct_param cfg=AnalysisConfig'
    → {"cfg": "AnalysisConfig"}
    """
    result = {}
    for line in doc.split("\n"):
        line = line.strip().lstrip("/").strip()
        if "+devlore:struct_param " in line:
            idx = line.index("+devlore:struct_param ")
            pairs = line[idx + len("+devlore:struct_param "):].strip()
            for pair in pairs.split(","):
                pair = pair.strip()
                if "=" in pair:
                    k, v = pair.split("=", 1)
                    result[k.strip()] = v.strip()
    return result

# =============================================================================
# Type Graph Helpers
# =============================================================================

def is_custom_return(returns):
    """Check if a method return type is a custom type (not a primitive).

    For '(*Sources, error)', returns 'Sources'.
    For '(string, error)', returns ''.
    """
    if returns in PRIMITIVE_RETURNS:
        return ""
    # Strip parens and error suffix: '(*Sources, error)' → '*Sources'
    r = returns
    if r.startswith("(") and r.endswith(")"):
        r = r[1:-1]
    if r.endswith(", error"):
        r = r[:-len(", error")]
    r = r.strip()
    # Strip pointer: '*Sources' → 'Sources'
    if r.startswith("*"):
        return r[1:]
    return ""

def filter_ctx_param(params):
    """Strip a leading framework-injected parameter from the params list.

    When a provider method's first Go parameter is one of the framework-injected types,
    [op.Method.Invoke] auto-fills it and the remaining parameters align with the caller-supplied parameter
    names. The announce map and starlark-facing surface must not list the injected parameter — it is implicit.

    Recognized framework-injected first parameters (mirror [op.NewMethod]'s detection):
      - *op.ActivationRecord — the per-dispatch record carrying Runtime, NodeID, Context (firstParamIsActivation).
      - context.Context      — the per-session cancellation context (legacy; predates ActivationRecord).
    """
    if len(params) > 0 and params[0].type in ("*op.ActivationRecord", "context.Context"):
        return params[1:]
    return params

def filter_methods(methods, include_list):
    """Filter methods down to the user-facing public surface.

    Excludes:
      - unexported methods (lowercase first letter)
      - framework methods listed in SKIP_METHODS
      - Compensate<Name> companions (discovered by reflection at runtime)
      - <Name>Planned companions (discovered by reflection at runtime)

    Compensate and Planned companions are not registered as standalone
    starlark-callable actions. They are attached to their forward method
    by methodFromReflectedMethod in pkg/op/receiver_type.go via naming-
    convention reflection lookup. See docs/architecture/4-resource-management.md
    §6.8 "Companion triplet".
    """
    filtered = []
    all_names = {}
    for m in methods:
        all_names[m.name] = True

    for m in methods:
        if m.name[0].islower():
            continue
        if m.name in SKIP_METHODS:
            continue
        if m.name.startswith("Compensate"):
            continue
        if m.name.endswith("Planned"):
            continue
        if include_list and m.name not in include_list:
            continue
        filtered.append(m)
    return filtered, all_names

def resolve_struct_param(struct_type, structs_by_name, path):
    """Resolve a struct_param type, handling cross-package references.

    For local types (no "."), looks up in structs_by_name.
    For cross-package types (e.g., "staranalysis.AnalysisConfig"), resolves
    by calling goast.structs() on the sibling package.

    Returns (struct_info, resolved_type) where resolved_type is the type name
    to use in generated code (may include package prefix for cross-package).
    """
    if "." not in struct_type:
        if struct_type not in structs_by_name:
            fail("struct_param type %s not found in package structs" % struct_type)
        return structs_by_name[struct_type], struct_type

    # Cross-package: "staranalysis.AnalysisConfig" → pkg="staranalysis", bare="AnalysisConfig"
    pkg_alias, bare = struct_type.split(".", 1)
    sibling_path = file.join(file.parent(path), pkg_alias)
    if not file.exists(sibling_path):
        fail("cross-package struct_param: sibling package path %s does not exist" % sibling_path)

    sibling_structs = goast.structs(sibling_path)
    for s in sibling_structs:
        if s.name == bare:
            return s, struct_type
    fail("struct_param type %s not found in sibling package %s" % (struct_type, sibling_path))

def build_method_descriptors(methods, all_names, defaults_map, struct_param_map, structs_by_name, path):
    """Build method descriptor dicts from filtered method list.

    defaults_map: method_name → {param_name: default_value}
    struct_param_map: method_name → {var_name: struct_type}
    structs_by_name: struct_name → struct info from goast.structs()
    path: filesystem path to the package (for cross-package struct resolution)
    """
    descriptors = []
    for m in methods:
        method_defaults = defaults_map.get(m.name, {})
        method_struct_params = struct_param_map.get(m.name, {})
        compensable = ("Compensate" + m.name) in all_names
        pure = "error" not in m.returns

        params = []
        for p in filter_ctx_param(m.params):
            # Struct param: emit the Go param name (not expanded fields).
            # The marshaler handles dict → struct conversion.
            if p.name in method_struct_params:
                params.append({
                    "name": p.name,
                    "type": method_struct_params[p.name],
                    "variadic": False,
                    "doc": "",
                    "optional": True,
                    "default": "",
                })
            else:
                default_val = method_defaults.get(p.name, "")
                is_variadic = p.variadic or (p.name == "args" and p.type.startswith("[]"))
                is_kwargs = p.name == "kwargs" and p.type.startswith("map[string]")
                # Variadic and **kwargs params are inherently optional — the caller may always omit positional
                # overflow or extra keyword args. Mirroring the runtime invariant in pkg/op/parameter.go where
                # parseParameterToken sets Parameter.Optional for these forms unconditionally.
                is_optional = is_variadic or is_kwargs or (p.name in method_defaults)
                params.append({
                    "name": p.name,
                    "type": p.type,
                    "variadic": is_variadic,
                    "kwargs": is_kwargs,
                    "doc": p.doc,
                    "optional": is_optional,
                    "default": default_val,
                })

        # Semantic validation of +devlore:defaults against this method's parameter list. Every name in
        # method_defaults must correspond to a real param on this method, and that param must not be variadic or
        # **kwargs (Q7 grammar — defaults bind only to named scalar params). The runtime parser
        # (pkg/op/parameter.go:parseParameterToken) repeats these checks as the contract gate, but failing here
        # surfaces the error at make build time with a precise file/method context.
        params_by_name = {p["name"]: p for p in params}
        for default_name in method_defaults:
            target = params_by_name.get(default_name)
            if target == None:
                fail("method %s: +devlore:defaults names %r but the method has no such parameter" % (m.name, default_name))
            if target.get("variadic"):
                fail("method %s: +devlore:defaults cannot apply to variadic parameter %r" % (m.name, default_name))
            if target.get("kwargs"):
                fail("method %s: +devlore:defaults cannot apply to **kwargs parameter %r" % (m.name, default_name))

        # Auto-detect property methods: no params and primitive return type.
        # These become read-only attributes (direct value, not callable).
        is_property = len(params) == 0 and not is_custom_return(m.returns)

        desc = {
            "name": m.name,
            "returns": m.returns,
            "doc": m.doc,
            "params": params,
            "compensable": compensable,
            "pure": pure,
            "property": is_property,
            "file": m.file,
            "line": m.line,
        }
        descriptors.append(desc)
    return descriptors

# =============================================================================
# Struct Converter Helpers
# =============================================================================

def go_type_to_kind(go_type):
    """Map a Go type string to a converter field kind."""
    if go_type == "string":
        return "string"
    if go_type == "int":
        return "int"
    if go_type == "int64":
        return "int64"
    if go_type == "bool":
        return "bool"
    if go_type == "[]string":
        return "string_slice"
    return ""

def cross_pkg_converter(pkg_alias, bare_type):
    """Build a cross-package converter function name: statstatsgen.StatsToStarlark."""
    return pkg_alias + "gen." + bare_type + "ToStarlark"

def cross_pkg_import_info(pkg_alias):
    """Build a cross-package import info dict for a sibling provider."""
    return {"alias": pkg_alias + "gen", "pkg": pkg_alias}

def build_converter_field(field, structs_by_name):
    """Build a single converter field descriptor from a struct field."""
    kind = go_type_to_kind(field.type)
    snake = to_snake(field.name)

    if kind:
        return {
            "go_name": field.name,
            "snake_name": snake,
            "kind": kind,
        }

    # Pointer to struct: *Stats or *starstats.Stats → struct_ptr
    if field.type.startswith("*"):
        inner = field.type[1:]
        if inner in structs_by_name:
            return {
                "go_name": field.name,
                "snake_name": snake,
                "kind": "struct_ptr",
                "converter": inner + "ToStarlark",
                "nullable": True,
                "nil_expr": "starlark.None",
            }
        if "." in inner:
            pkg_alias, bare = inner.split(".", 1)
            return {
                "go_name": field.name,
                "snake_name": snake,
                "kind": "struct_ptr",
                "converter": cross_pkg_converter(pkg_alias, bare),
                "nullable": True,
                "nil_expr": "starlark.None",
                "cross_pkg_import": cross_pkg_import_info(pkg_alias),
            }

    # Slice of struct: []T or []pkg.T → struct_slice
    if field.type.startswith("[]"):
        elem = field.type[2:]
        if elem in structs_by_name:
            return {
                "go_name": field.name,
                "snake_name": snake,
                "kind": "struct_slice",
                "converter": elem + "ToStarlark",
            }
        if "." in elem:
            pkg_alias, bare = elem.split(".", 1)
            return {
                "go_name": field.name,
                "snake_name": snake,
                "kind": "struct_slice",
                "converter": cross_pkg_converter(pkg_alias, bare),
                "cross_pkg_import": cross_pkg_import_info(pkg_alias),
            }

    # Direct struct value: T or pkg.T → struct_value
    if field.type in structs_by_name:
        return {
            "go_name": field.name,
            "snake_name": snake,
            "kind": "struct_value",
            "converter": field.type + "ToStarlark",
        }
    if "." in field.type:
        pkg_alias, bare = field.type.split(".", 1)
        return {
            "go_name": field.name,
            "snake_name": snake,
            "kind": "struct_value",
            "converter": cross_pkg_converter(pkg_alias, bare),
            "cross_pkg_import": cross_pkg_import_info(pkg_alias),
        }

    return None

def collect_pointer_types(all_data_structs, structs_by_name, dependent_descriptors, provider_methods):
    """Collect all struct types that are referenced as pointers.

    A type needs a pointer receiver in its converter if it appears as:
    - *T return type from a method (dependent type or provider method)
    - *T field in another struct (struct_ptr kind)
    """
    pointer_types = {}

    # From provider method returns: (*T, error) means T is pointer-referenced.
    for desc in provider_methods:
        ret = is_custom_return(desc["returns"])
        if ret and ret in all_data_structs:
            pointer_types[ret] = True

    # From dependent type method returns: (*T, error) means T is pointer-referenced.
    for type_name, descs in dependent_descriptors.items():
        for desc in descs:
            ret = is_custom_return(desc["returns"])
            if ret and ret in all_data_structs:
                pointer_types[ret] = True

    # From struct fields: *T fields mark T as pointer-referenced.
    for struct_name in all_data_structs:
        if struct_name not in structs_by_name:
            continue
        info = structs_by_name[struct_name]
        for field in info.fields:
            if field.type.startswith("*"):
                inner = field.type[1:]
                if inner in all_data_structs:
                    pointer_types[inner] = True

    return pointer_types

def build_converter(struct_name, structs_by_name, pointer_types):
    """Build a converter descriptor for a struct type."""
    if struct_name not in structs_by_name:
        return None

    info = structs_by_name[struct_name]
    fields = []
    for field in info.fields:
        fd = build_converter_field(field, structs_by_name)
        if fd:
            fields.append(fd)

    is_pointer = struct_name in pointer_types
    func_name = struct_name + "ToStarlark"
    starlark_name = to_snake(struct_name)

    return {
        "func_name": func_name,
        "go_type": struct_name,
        "is_pointer": is_pointer,
        "starlark_name": starlark_name,
        "fields": fields,
    }

def collect_type_graph(path, provider_methods, structs_by_name):
    """Walk the type graph starting from Provider methods.

    Returns:
      - dependent_types: list of type names that need HasAttrs wrappers (have methods)
      - data_structs: set of type names that need struct converters (no methods, just data)
    """
    # Find custom return types from Provider methods
    custom_returns = []
    for desc in provider_methods:
        type_name = is_custom_return(desc["returns"])
        if type_name:
            custom_returns.append(type_name)

    dependent_types = []
    data_structs = {}
    seen = {}

    def walk_return_type(type_name):
        if type_name in seen:
            return
        seen[type_name] = True

        # Resource types are handled by the resource template path, not dependent_type.
        if type_name == "Resource":
            return

        # Check if this type has methods (→ dependent type with HasAttrs wrapper)
        type_methods = goast.methods(path, receiver_type=type_name)
        has_methods = False
        for m in type_methods:
            if m.name[0].isupper() and m.name not in SKIP_METHODS:
                has_methods = True
                break

        if has_methods:
            dependent_types.append(type_name)
            # Walk this type's methods for further custom returns
            filtered, _ = filter_methods(type_methods, [])
            for m in filtered:
                sub_type = is_custom_return(m.returns)
                if sub_type:
                    walk_return_type(sub_type)
        else:
            # Data struct — needs converter only
            if type_name in structs_by_name:
                data_structs[type_name] = True

    for t in custom_returns:
        walk_return_type(t)

    return dependent_types, data_structs

def collect_all_data_structs(dependent_descriptors, data_structs, structs_by_name):
    """Collect all data structs referenced by dependent type methods.

    Walks method returns of dependent types plus transitive struct field references.
    """
    # Start with directly referenced data structs
    all_data = dict(data_structs)

    # Add data structs from dependent type method returns
    for descs in dependent_descriptors.values():
        for desc in descs:
            type_name = is_custom_return(desc["returns"])
            if type_name and type_name in structs_by_name and type_name not in dependent_descriptors:
                all_data[type_name] = True

    # Transitively walk struct fields to find nested struct references.
    # Use iterative expansion since Starlark has no while loops.
    queue = list(all_data.keys())
    for _ in range(100):  # safety limit for transitive closure
        if not queue:
            break
        current = queue[0]
        queue = queue[1:]
        if current not in structs_by_name:
            continue
        info = structs_by_name[current]
        for field in info.fields:
            # Slice of struct: []T
            if field.type.startswith("[]"):
                elem = field.type[2:]
                if elem in structs_by_name and elem not in all_data:
                    all_data[elem] = True
                    queue.append(elem)
            # Pointer to struct: *T
            elif field.type.startswith("*"):
                elem = field.type[1:]
                if elem in structs_by_name and elem not in all_data:
                    all_data[elem] = True
                    queue.append(elem)
            # Direct struct embed: T (if it's a known struct)
            elif field.type in structs_by_name and field.type not in all_data:
                inner = field.type
                all_data[inner] = True
                queue.append(inner)

    return all_data

# =============================================================================
# Resource Detection
# =============================================================================

def detect_resources(path):
    """Detect every Resource type in the package, paired with its constructor.

    A Resource type is identified by its public constructor: any function in the package whose
    signature is `func(*op.RuntimeEnvironment, any) (*T, error)` or `(T, error)` declares T as
    a Resource. The constructor IS the public contract — embedding chains can be transitive
    (e.g., mem.Function embeds mem.Resource which embeds op.ResourceBase) and structural-only
    detection misses those; constructor-signature detection catches every type the package
    publicly exposes as a Resource.

    Returns a list of (struct_name, constructor_name) tuples — one entry per detected Resource.
    Returns the empty list if no matching constructors are found. Fails if multiple constructors
    return the same type.
    """
    funcs = goast.funcs(path)

    results = []
    seen_types = {}
    for fn in funcs:
        type_name = _resource_return_type(fn)
        if not type_name:
            continue
        if type_name in seen_types:
            fail("multiple constructors found for Resource type %s: %s and %s" % (type_name, seen_types[type_name], fn.name))
        seen_types[type_name] = fn.name
        results.append((type_name, fn.name))
    return results

def _resource_return_type(fn):
    """Return the Resource type name fn constructs, or "" if fn isn't a Resource constructor.

    A Resource constructor is `func(*op.RuntimeEnvironment, any) (*T, error)` or `(T, error)`.
    Returns the bare type name T (no leading `*`).
    """
    if len(fn.params) != 2:
        return ""
    if fn.params[0].type != "*op.RuntimeEnvironment":
        return ""
    if fn.params[1].type not in ["any", "interface{}"]:
        return ""
    ret = fn.returns
    if not ret.startswith("(") or not ret.endswith(", error)"):
        return ""
    inner = ret[1:-len(", error)")]
    if inner.startswith("*"):
        inner = inner[1:]
    return inner

def detect_resource_params(path, struct_name):
    """Detect parameterized methods on the named Resource struct.

    Finds exported methods on *struct_name that take parameters and return (T) or (T, error).
    Methods returning only error are excluded (not useful as Starlark callables). Methods with
    unnamed parameters (_) are excluded (cannot be called by name from Starlark).

    Parameters:
      - path:        the package path.
      - struct_name: the Resource type's struct name (e.g., "Resource", "Function").

    Returns:
      list of {"name": GoName, "params": [snake_name, ...]} dicts, or [] if none found.
    """
    methods = goast.methods(path, receiver_type=struct_name)
    result = []
    for m in methods:
        if m.name[0].islower():
            continue
        if m.name in SKIP_METHODS:
            continue
        if not m.params:
            continue
        # Reject error-only returns.
        if m.returns in ["error", "(error)"]:
            continue
        # Accept (T) or (T, error) returns only.
        ret = m.returns
        if ret.startswith("(") and ret.endswith(")"):
            inner = ret[1:-1]
            parts = [p.strip() for p in inner.split(",")]
            if len(parts) > 2:
                continue
            if len(parts) == 2 and parts[1] != "error":
                continue
        # Skip methods with unnamed parameters.
        has_unnamed = False
        param_names = []
        for p in filter_ctx_param(m.params):
            if p.name == "_" or not p.name:
                has_unnamed = True
                break
            param_names.append(to_snake(p.name))
        if has_unnamed:
            continue
        result.append({"name": m.name, "params": param_names})
    return result

# =============================================================================
# Generation: Gen/ Mode
# =============================================================================

def compute_provider_import(path):
    """Compute the Go import path for the provider package.

    Uses goast.deps() to get the module path, then finds go.mod to compute
    the relative package path.
    """
    deps = goast.deps(path)
    module_path = deps.module_path

    if not module_path:
        fail("could not detect Go module path for " + path)

    # Walk up from path to find go.mod directory
    go_mod_dir = ""
    dir = path
    for _ in range(20):  # safety limit
        if file.exists(file.join(dir, "go.mod")):
            go_mod_dir = dir
            break
        parent = file.parent(dir)
        if parent == dir:
            break
        dir = parent

    if not go_mod_dir:
        fail("could not find go.mod for " + path)

    # Compute relative path from go.mod dir to the provider package.
    if go_mod_dir == "." or go_mod_dir == "":
        rel = path
    elif path.startswith(go_mod_dir + "/"):
        rel = path[len(go_mod_dir) + 1:]
    elif path == go_mod_dir:
        rel = ""
    else:
        fail("provider path %s is not under module root %s" % (path, go_mod_dir))

    if rel:
        return module_path + "/" + rel
    return module_path

def emit_provider_receiver(command, path, provider, struct_short, struct_name, access, lifetime, root,
                      all_method_names, provider_descriptors,
                      output_dir, write_files):
    """Generate receivers in gen/ mode with type graph walking."""

    pkg = provider
    provider_import = compute_provider_import(path)
    ui.note("Provider import: " + provider_import)

    # -------------------------------------------------------------------------
    # Require ProviderBase embedding
    # -------------------------------------------------------------------------
    embeds_provider_base = False
    structs = goast.structs(path)
    for s in structs:
        if s.name == "Provider":
            for f in s.fields:
                if f.embedded and f.type == "op.ProviderBase":
                    embeds_provider_base = True
    if not embeds_provider_base:
        fail("Provider struct must embed op.ProviderBase")

    # -------------------------------------------------------------------------
    # Parse defaults and struct_param directives from method docs
    # -------------------------------------------------------------------------
    structs = goast.structs(path)
    structs_by_name = {}
    for s in structs:
        structs_by_name[s.name] = s

    # Build defaults_map and struct_param_map for Provider methods
    defaults_map = {}
    struct_param_map = {}
    for desc in provider_descriptors:
        method_defaults = parse_defaults(desc["doc"], desc["name"])
        if method_defaults:
            defaults_map[desc["name"]] = method_defaults
        method_struct_params = parse_struct_param(desc["doc"])
        if method_struct_params:
            struct_param_map[desc["name"]] = method_struct_params

    # -------------------------------------------------------------------------
    # Walk type graph to find dependent types and data structs
    # -------------------------------------------------------------------------
    dependent_types, data_structs = collect_type_graph(path, provider_descriptors, structs_by_name)
    ui.note("Dependent types: " + str(dependent_types))
    ui.note("Data structs: " + str(list(data_structs.keys())))

    # -------------------------------------------------------------------------
    # Build dependent type method descriptors
    # -------------------------------------------------------------------------
    dependent_descriptors = {}
    for type_name in dependent_types:
        type_methods = goast.methods(path, receiver_type=type_name)
        filtered, dep_all_names = filter_methods(type_methods, [])

        # Parse defaults and struct_param for dependent type methods
        dep_defaults = {}
        dep_struct_params = {}
        for m in filtered:
            md = parse_defaults(m.doc, m.name)
            if md:
                dep_defaults[m.name] = md
            ms = parse_struct_param(m.doc)
            if ms:
                dep_struct_params[m.name] = ms

        descs = build_method_descriptors(filtered, dep_all_names, dep_defaults, dep_struct_params, structs_by_name, path)
        dependent_descriptors[type_name] = descs

    # -------------------------------------------------------------------------
    # Collect all data structs (transitively)
    # -------------------------------------------------------------------------
    all_data_structs = collect_all_data_structs(dependent_descriptors, data_structs, structs_by_name)
    ui.note("All data structs for converters: " + str(list(all_data_structs.keys())))

    # Data struct returns are handled by WrapReceiver's auto-bridging via
    # classifyReturn → marshalReflect → marshalStruct. No converter annotation needed.

    # -------------------------------------------------------------------------
    # Re-build Provider method descriptors with defaults/struct_param applied
    # -------------------------------------------------------------------------
    all_methods_raw = goast.methods(path, receiver_type=struct_name)
    filtered_raw, all_names_raw = filter_methods(all_methods_raw, [])
    provider_method_descs = build_method_descriptors(
        filtered_raw, all_names_raw, defaults_map, struct_param_map, structs_by_name, path,
    )

    # Data struct returns are handled by WrapReceiver's auto-bridging via
    # classifyReturn → marshalReflect → marshalStruct. No converter annotation needed.

    # -------------------------------------------------------------------------
    # Generate: Provider immediate receiver (gen/immediate.gen.go)
    # -------------------------------------------------------------------------
    # Prefix struct_type with "provider." for gen/ subpackage mode.
    # Cross-package struct types (containing ".") keep their qualifier as-is.
    for d in provider_method_descs:
        for p in d.get("params", []):
            st = p.get("struct_type", "")
            if st and "." not in st:
                p["struct_type"] = "provider." + st

    namespace = provider
    if access == "planned":
        # Planned providers also get immediate for gen/ mode
        namespace = "plan." + provider

    # Collect cross-package imports from provider method result_exprs and struct_params
    provider_cross_imports = collect_cross_pkg_imports(provider_import, [], [provider_method_descs])

    provider_desc = {
        "package": pkg,
        "provider": provider,
        "struct_name": struct_short,
        "namespace": namespace,
        "impl_type": struct_name,
        "registered": True,
        "provider_import": provider_import,
        "methods": provider_method_descs,
        "all_methods": list(all_names_raw.keys()),
        "access": access,
        "access_title": access_title(access),
        "root": root,
        "lifetime": lifetime,
        "lifetime_title": lifetime_title(lifetime),
    }
    if provider_cross_imports:
        provider_desc["cross_package_imports"] = provider_cross_imports

    emit_file(command, "provider", provider_desc, "gen/provider.gen.go",
             struct_short, len(provider_method_descs), output_dir, write_files)

    # Generate receiver type tests (always — type descriptor exists for all providers).
    emit_file(command, "receiver_type_test", provider_desc, "gen/receiver_type.gen_test.go",
             struct_short, len(provider_method_descs), output_dir, write_files)

    # Generate module tests (starlark module protocol).
    if access in ["immediate", "both"]:
        emit_file(command, "module_test", provider_desc, "gen/module.gen_test.go",
                 struct_short, len(provider_method_descs), output_dir, write_files)

    # Generate action tests (action wrappers — dry-run, compensable, undo).
    if access in ["planned", "both"]:
        emit_file(command, "action_test", provider_desc, "gen/action.gen_test.go",
                 struct_short, len(provider_method_descs), output_dir, write_files)

    # Generate plan-adapter tests (plan adapter — node creation from starlark calls).
    if access in ["planned", "both"]:
        emit_file(command, "node_builder_test", provider_desc, "gen/node_builder.gen_test.go",
                 struct_short, len(provider_method_descs), output_dir, write_files)

    generated_count = 1

    # -------------------------------------------------------------------------
    # Dependent type receivers (gen/<type_snake>.gen.go)
    # -------------------------------------------------------------------------
    for type_name in dependent_types:
        type_snake = to_snake(type_name)
        dep_descs = dependent_descriptors.get(type_name, [])
        dep_desc = {
            "package": pkg,
            "provider": provider,
            "provider_import": provider_import,
            "provider_type_prefix": "provider.",
            "type_name": type_name,
            "starlark_name": type_snake,
            "methods": dep_descs,
        }
        dep_filename = "gen/" + type_snake + ".gen.go"
        emit_file(command, "dependent_type", dep_desc, dep_filename,
                 type_name, len(dep_descs), output_dir, write_files)

    # Struct converters are no longer generated — op.Marshal handles all
    # struct-to-Starlark conversion via reflection.

    # -------------------------------------------------------------------------
    # Generate: Resource descriptors — one gen file per Resource type in the package.
    # -------------------------------------------------------------------------
    for struct_name, constructor_name in detect_resources(path):
        snake = to_snake(struct_name)
        resource_params = detect_resource_params(path, struct_name)
        resource_desc = {
            "package": pkg,
            "provider": provider,
            "provider_import": provider_import,
            "provider_type_prefix": "provider.",
            "struct_name": struct_name,
            "constructor_name": constructor_name,
            "resource_params": resource_params,
        }
        emit_file(command, "resource", resource_desc, "gen/" + snake + ".gen.go",
                 struct_name, 1, output_dir, write_files)
        generated_count += 1

    ui.succeed("Done. Generated %d file(s) in gen/ mode for %s" % (generated_count, struct_short))

def collect_cross_pkg_imports(provider_import, converters, method_desc_lists):
    """Collect cross-package imports from converter fields, method result_exprs, and struct_params.

    Returns a list of {"alias": "starstatsgen", "path": "github.com/.../starstats/gen"}
    or {"alias": "staranalysis", "path": "github.com/.../staranalysis"} for struct params.
    """
    if "/" not in provider_import:
        return []

    base = provider_import.rsplit("/", 1)[0]  # e.g., ".../pkg/op/provider"
    imports = {}

    # From converter fields with cross_pkg_import info
    for conv in converters:
        for field in conv.get("fields", []):
            cpkg = field.get("cross_pkg_import")
            if cpkg and cpkg["alias"] not in imports:
                imports[cpkg["alias"]] = base + "/" + cpkg["pkg"] + "/gen"

    # From method descriptors with cross-package result_expr
    for desc_list in method_desc_lists:
        for desc in desc_list:
            expr = desc.get("result_expr", "")
            if "gen." in expr:
                # Extract alias from e.g. "starstatsgen.StatsToStarlark(%s)"
                alias = expr.split(".")[0]
                if alias.endswith("gen") and alias not in imports:
                    pkg = alias[:-3]
                    imports[alias] = base + "/" + pkg + "/gen"

            # From struct_param cross-package types (raw package import, not gen/)
            for p in desc.get("params", []):
                st = p.get("struct_type", "")
                if st and "." in st and "provider." not in st:
                    pkg_alias = st.split(".", 1)[0]
                    if pkg_alias not in imports:
                        imports[pkg_alias] = base + "/" + pkg_alias

    result = []
    for alias in sorted(imports.keys()):
        result.append({"alias": alias, "path": imports[alias]})
    return result

def annotate_result_exprs(descriptors, data_structs, provider_prefix):
    """Set result_expr on methods whose return type is a data struct or cross-package type.

    Local data structs use converter functions (e.g., IndexToStarlark(result)).
    Cross-package types use qualified converter calls (e.g., starindexgen.IndexToStarlark(result)).

    provider_prefix: if non-empty, prefixed to converter calls for gen/ mode
    (currently not needed since converters live in same package).
    """
    for desc in descriptors:
        type_name = is_custom_return(desc["returns"])
        if type_name and "." in type_name:
            # Cross-package type: starindex.Index → starindexgen.IndexToStarlark
            pkg_alias, bare = type_name.split(".", 1)
            converter = cross_pkg_converter(pkg_alias, bare)
            desc["result_expr"] = converter + "(%s)"
        elif type_name and type_name in data_structs:
            converter = type_name + "ToStarlark"
            desc["result_expr"] = converter + "(%s)"

# =============================================================================
# Pre-computation Helpers for goast.render()
# =============================================================================

def compute_provider_type_prefix(desc):
    """Return 'provider.' for gen/ subpackage mode, '' for same-package."""
    if desc.get("provider_import", ""):
        return "provider."
    return ""

def compute_param_names_list(method):
    """Pre-compute the quoted, comma-separated parameter name list for a method.

    Token grammar emitted to the runtime:
      ("**" | "*")? name ("?" ("=" defaultExpr)?)?

    Branch order: kwargs > variadic > default > optional. A param with both
    "default" and "optional" set takes the default branch — "?=value"
    already encodes optional via the "?". The runtime parses these tokens
    in pkg/op/parameter.go:parseParameterToken.

    A "default" value is emitted inline only when the param's Go type is
    one of the runtime-supported defaultable kinds (bool, int*, uint*,
    float*, string, or a named type whose underlying is one of these).
    Composite types (slice, map, pointer, interface, channel, function)
    fall through to the "?" branch — the runtime's parseDefaultExpression
    cannot parse a literal text against them, and historically these
    directives carry markers like "nil" / "[]" that mean "use Go zero
    value" rather than a real default.
    """
    parts = []
    for p in method.get("params", []):
        name = to_snake(p["name"])
        default = p.get("default", "")
        if p.get("kwargs"):
            name = "**" + name
        elif p.get("variadic"):
            name = "*" + name
        elif default.startswith("{{") and default.endswith("}}"):
            # Deferred-default expression — evaluated at slot-fill via op.DeferredDefault. Bypass the
            # is_simple_defaultable_type filter (the runtime evaluator handles any target type via
            # op.Convert at slot-fill, not parseDefaultExpression's reflect.Kind dispatch). Emit the
            # literal {{ ... }} text verbatim, Go-string-escaped for embedding in the announce-map's
            # Go source string literal.
            escaped = default.replace("\\", "\\\\").replace("\"", "\\\"")
            name += "?=" + escaped
        elif default and is_simple_defaultable_type(p.get("type", "")):
            # Go-string-escape the default expression: backslash first (so subsequent escapes don't double-back),
            # then double-quote. Preserves literal quotes from directives like `severity="warning"` when the
            # token is embedded in a Go source string literal.
            escaped = default.replace("\\", "\\\\").replace("\"", "\\\"")
            name += "?=" + escaped
        elif default or p.get("optional"):
            name += "?"
        parts.append('"' + name + '"')
    return ", ".join(parts)

def is_simple_defaultable_type(go_type):
    """Return True if go_type is structurally suitable for parseDefaultExpression.

    The runtime helper at pkg/op/parameter.go:parseDefaultExpression dispatches
    by reflect.Kind across bool / int* / uint* / float* / string. Composite
    Go types (slice, map, pointer, interface, channel, function, builtin
    "any") are not defaultable from a literal text token; codegen drops them
    to the optional-only "?" form rather than emitting "?=value" the runtime
    cannot parse.
    """
    if go_type.startswith("[]"):
        return False
    if go_type.startswith("map["):
        return False
    if go_type.startswith("*"):
        return False
    if go_type.startswith("chan"):
        return False
    if go_type.startswith("func"):
        return False
    if go_type == "interface{}" or go_type == "any":
        return False
    return True

def compute_provider_init(desc):
    """Pre-compute the ImmediateFactory body code.

    Generates the Go code that constructs an empty provider and delegates to New<StructName><WrapperSuffix>.
    """
    prefix = compute_provider_type_prefix(desc)
    struct_name = desc["struct_name"]
    wrapper_suffix = desc.get("wrapper_suffix", "Receiver")

    return "\t\t\treturn New%s%s(&%sProvider{})" % (struct_name, wrapper_suffix, prefix)

def compute_descriptor_init(desc):
    """Pre-compute the NewImmediate method body for the provider descriptor.

    Same shape as compute_provider_init but with single-tab indentation (method body level, not nested inside a
    closure).
    """
    prefix = compute_provider_type_prefix(desc)
    struct_name = desc["struct_name"]
    wrapper_suffix = desc.get("wrapper_suffix", "Receiver")

    return "\treturn New%s%s(&%sProvider{})" % (struct_name, wrapper_suffix, prefix)

def prepare_render_data(descriptor, template_name):
    """Prepare a descriptor dict for goast.render().

    Pre-computes template function values and adds derived fields.
    Returns render_data.
    """
    # Shallow copy to avoid mutating the original
    desc = dict(descriptor)

    # Apply defaults for optional fields
    if not desc.get("wrapper_suffix", ""):
        desc["wrapper_suffix"] = "Receiver"

    # Pre-compute provider type prefix
    desc["provider_type_prefix"] = compute_provider_type_prefix(desc)

    # Pre-compute descriptor fields for provider template
    if template_name == "provider":
        access = desc.get("access", "immediate")
        root = desc.get("root", False)
        desc["has_actions"] = access in ["planned", "both"]
        desc["has_planned"] = access in ["planned", "both"]
        desc["has_immediate"] = access in ["immediate", "both"]
        if access == "immediate":
            roles = "op.RoleModule"
        elif access == "planned":
            roles = "op.RoleAction"
        else:
            roles = "op.RoleModule|op.RoleAction"
        if root:
            roles = roles + "|op.RoleRoot"
        desc["roles"] = roles

    # Add derived fields to each method
    methods = list(desc.get("methods", []))
    enriched = []
    for m in methods:
        md = dict(m)
        md["snake_name"] = to_snake(m["name"])
        md["param_names_list"] = compute_param_names_list(m)
        enriched.append(md)
    desc["methods"] = enriched

    return desc

def emit_file(command, template_name, descriptor, filename, label, method_count, output_dir, write_files):
    """Generate a single file from a template and descriptor."""
    ui.note("Generating %s for %s (%d items)..." % (template_name, label, method_count))
    template_content = load_template(template_name, command.extension.dir)

    # Pre-compute template values and render via goast.render()
    render_data = prepare_render_data(descriptor, template_name)
    code = goast.render(template=template_content, data=render_data)

    if write_files and output_dir:
        out_path = output_dir + "/" + filename
        # Ensure gen/ subdirectory exists. Explicit modes pending 13.0(f) step 12 (umask deferred-default);
        # without them the slot defaults to FileMode(0) and the written files become inaccessible.
        out_dir = file.parent(out_path)
        if not file.exists(out_dir):
            file.mkdir(out_dir, chmod = 0o755)
        file.write_text(out_path, code, chmod = 0o644)
        ui.succeed("Wrote " + out_path)
    else:
        ui.note("--- " + filename + " ---")
        ui.note(code)

# =============================================================================
# Entry Point
# =============================================================================

def run(command, ctx):
    """Generate receivers and actions from a provider struct."""

    # -------------------------------------------------------------------------
    # Validate required arguments
    # -------------------------------------------------------------------------
    path = ctx.args.get("source", "").rstrip("/")
    if not path:
        fail("--source is required")

    gen_mode = ctx.args.get("gen", False)

    # All providers use the same struct name
    struct_name = "Provider"

    # -------------------------------------------------------------------------
    # Discover Provider methods (may be absent for resource-only packages)
    # -------------------------------------------------------------------------
    methods = goast.methods(path, receiver_type=struct_name)
    has_provider = len(methods) > 0

    if has_provider:
        filtered, all_method_names = filter_methods(methods, [])
        if len(filtered) == 0:
            fail("no eligible methods after filtering for " + struct_name)
    else:
        # No Provider struct — check for Resource structs before failing.
        resources = detect_resources(path)
        if not resources:
            fail("no Provider struct and no Resource struct in " + path)

        # Resource-only package: emit one gen file per detected Resource type, named after the
        # type (e.g., Resource → gen/resource.gen.go, Function → gen/function.gen.go). The
        # template is parameterized by struct_name + constructor_name; same template, multiple
        # outputs. The Makefile rule for the package must list every output as a grouped target.
        if not gen_mode:
            fail("--gen is required")
        output_dir = ctx.args.get("output", "")
        write_files = ctx.args.get("write", False)
        provider = path.split("/")[-1]
        provider_import = compute_provider_import(path)
        for struct_name, constructor_name in resources:
            snake = to_snake(struct_name)
            resource_params = detect_resource_params(path, struct_name)
            resource_desc = {
                "package": provider,
                "provider": provider,
                "provider_import": provider_import,
                "provider_type_prefix": "provider.",
                "struct_name": struct_name,
                "constructor_name": constructor_name,
                "resource_params": resource_params,
            }
            emit_file(command, "resource", resource_desc, "gen/" + snake + ".gen.go",
                     struct_name, 1, output_dir, write_files)
        ui.succeed("Done. Generated %d resource descriptor(s) for %s" % (len(resources), provider))
        return

    ui.note("Found " + str(len(filtered)) + " methods for " + struct_name)

    # -------------------------------------------------------------------------
    # Derive names and access/lifetime from struct directives
    # -------------------------------------------------------------------------
    provider = path.split("/")[-1]
    struct_short = provider.title()
    access = struct_access(path)
    lifetime = struct_lifetime(path)
    root = struct_root(path)

    ui.note("Provider access: " + access)
    if root:
        ui.note("Provider root: true")

    # -------------------------------------------------------------------------
    # Build basic method descriptors (without defaults/struct_param expansion)
    # -------------------------------------------------------------------------
    all_descriptors = []

    for m in filtered:
        params = []
        for p in filter_ctx_param(m.params):
            # Infer *args from variadic (...T) or slice ([]T) params.
            # Infer **kwargs from map[string]any params.
            is_variadic = p.variadic or (p.type.startswith("[]") and not p.type.startswith("[]byte"))
            is_kwargs = p.type == "map[string]any"
            params.append({
                "name": p.name,
                "type": p.type,
                "variadic": is_variadic,
                "kwargs": is_kwargs,
                "doc": p.doc,
            })
        compensable = ("Compensate" + m.name) in all_method_names
        pure = "error" not in m.returns

        desc = {
            "name": m.name,
            "returns": m.returns,
            "doc": m.doc,
            "params": params,
            "compensable": compensable,
            "pure": pure,
            "file": m.file,
            "line": m.line,
        }
        all_descriptors.append(desc)

    # -------------------------------------------------------------------------
    # Parse common flags
    # -------------------------------------------------------------------------
    output_dir = ctx.args.get("output", "")
    write_files = ctx.args.get("write", False)

    # -------------------------------------------------------------------------
    # Dispatch to gen/ mode
    # -------------------------------------------------------------------------
    if not gen_mode:
        fail("--gen is required")

    emit_provider_receiver(command, path, provider, struct_short, struct_name, access, lifetime, root,
                      all_method_names, all_descriptors,
                      output_dir, write_files)
