# glow: библиотека проекта Granio

**Исходный репозиторий:** [go-gl/glow](https://github.com/go-gl/glow).
**Цель форка:** Воспроизводимая генерация обёрток OpenGL/GLES для Granio и согласование шаблонов с библиотекой gl.

Репозиторий развивается самостоятельно. Исходная история, лицензия и авторство
сохранены. Минимальная версия Go — **1.27.1**. Собственный адрес модуля:
`github.com/Uzer007-admin/glow`
.

Платформенные ограничения и результаты проверок:
[развитие графического стека](https://github.com/Uzer007-admin/granio/blob/dev/granio/docs/NATIVE_STACK_IMPLEMENTATION.md).
Длительные проверки производительности в этот этап не входят.

---

## Документация исходного проекта и прежних этапов

Адреса установки в сохранённых примерах ниже могут относиться к исходнику.

# Glow

Glow is an OpenGL binding generator for Go. Glow parses the [OpenGL XML API registry](https://github.com/KhronosGroup/OpenGL-Registry/tree/master/xml) and the [EGL XML API registry](https://github.com/KhronosGroup/EGL-Registry/tree/master/api) to produce a machine-generated cgo bridge between Go functions and native OpenGL functions. Glow is a fork of [GoGL2](https://github.com/chsc/gogl2).

Features:

- Go functions that mirror the C specification using Go types.
- Support for multiple OpenGL APIs (GL/GLES/EGL/WGL/GLX/EGL), versions, and profiles.
- Support for extensions (including debug callbacks).
- Support for overloads to provide Go functions with different parameter signatures.

See the [open issues](https://github.com/go-gl/glow/issues) for caveats about the current state of the implementation.

## Generated Packages

Generated OpenGL binding packages are available in the [go-gl/gl](https://github.com/go-gl/gl) repository.

## Overloads

See subdirectory `xml/overload` for examples. The motivation here is to provide Go functions with different parameter signatures of existing OpenGL functions.

For example, `glVertexAttribPointer(..., void *)` cannot be used with `gl.VertexAttribPointer(..., unsafe.Pointer)` when using arbitrary offset values. The `checkptr` safeguard will abort the program when doing so.
Overloads allow the creation of an additional `gl.VertexAttribPointerWithOffset(..., uintptr)`, which calls the original OpenGL function with appropriate casts.

## Custom Packages

If the prebuilt, go-gettable packages are not suitable for your needs you can build your own. For example,

    go get github.com/go-gl/glow
    cd $GOPATH/src/github.com/go-gl/glow
    go build
    ./glow download
    ./glow generate -api=gl -version=3.3 -profile=core -remext=GL_ARB_cl_event
    go install ./gl-core/3.3/gl

**NOTE:** You will have to provide a GitHub token ([personal access or OAuth2 token](https://developer.github.com/v3/auth/#basic-authentication)) to update the XML specification files.

A few notes about the flags to `generate`:

- `api`: One of `gl`, `gles1`, `gles2`, `egl`, `wgl`, or `glx`.
- `version`: The API version to generate. The `all` pseudo-version includes all functions and enumerations for the specified API.
- `profile`: For `gl` packages with version 3.2 or higher, `core` or `compatibility` ([explanation](http://www.opengl.org/wiki/Core_And_Compatibility_in_Contexts)).
- `xml`: The XML directory.
- `tmpl`: The template directory.
- `out`: The output directory for generated files.
- `addext`: If non-empty, a regular expression describing which extensions to include _in addition_ to those supported by the selected profile. Empty by default, including nothing additional. Takes precedence over explicit removal.
- `remext`: If non-empty, a regular expression describing which extensions to exclude. Empty by default, excluding nothing.
- `restrict`: A JSON file that explicitly lists what enumerations / functions that Glow should generate (see example.json).
  Unknown JSON fields, unknown symbols and duplicate symbols are errors, checked
  before generation. Omitted lists retain all symbols of that category; an
  explicit empty list (`"functions": []` or `"enums": []`) generates none.
- `lenientInit`: Flag to disable strict function availability checks at `Init` time. By default if any non-extension function pointer cannot be loaded then initialization fails; when this flag is set initialization will succeed with missing functions. Note that on some platforms unavailable functions will load successfully even but fail upon invocation so check against the OpenGL context what is supported.
- `dispatch`: Also generate immutable per-context `Functions` tables and
  `Load`/`LoadWithProcAddrFunc`. Legacy globals remain available. Profiles with
  debug callbacks using global user data are rejected before output is touched.
  The manifest records `context_dispatch: true`. Tables include safe offset
  overloads and `ShaderSourceString`, which passes length-delimited source through
  a C-stack pointer array without C allocation. Context lifetime/thread rules
  still apply; recreate the table after context loss/recreation.

Dispatch profiles containing `glGetIntegerv` also get `GetFramebufferViewport`
and `GetTargetBindings`. They query only known fixed-size state through one
cgo transition and C stack outputs; they do not accept arbitrary output enums
that might write beyond a scalar/array. Separate read/draw FBO and pixel-unpack
queries are optional flags that require the corresponding context capability.
These are convenience bridges, not new required GL entry points or a substitute
for version/extension checks. Adapters without them can retain individual queries.

Generation writes and formats the generated Go files in an isolated staging
directory before publishing them. Template and formatting failures preserve the
previous generated Go files and manifest. Publication errors restore files
already replaced in that run; this is not a crash-atomic filesystem transaction.
Caller-owned Go files in the output directory are not formatted. A stale
`debug.go` is removed only when it bears Glow's generated-code marker; an
unmarked file blocks its deletion. Temporary staging directories are removed.
Do not run concurrent generators against the same output directory.

Every generated package receives a sorted `symbol_manifest.json` (schema 2)
containing API/version/profile, enums and procedure names with their Go names and
required flags. Generated `IsFunctionAvailable` queries report addresses only;
context version/extension checks remain the consumer's responsibility.

Schema 2 retains the original fields and adds each function's return and ordered
parameter C/Go types, including pointer types. Identical generated Go files,
manifests, headers and vendor stubs keep their timestamps. Header copying reads
the source before touching the destination and stages changed writes, so a
missing source cannot truncate a working header. This does not provide one
crash-atomic transaction across the whole generated package and headers.
