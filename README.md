# Snippets.run CLI Architecture & Specification

Snippets.run is a version-controlled, executable snippet registry backed by Git. The CLI resolves, downloads, prepares, and executes remote code snippets locally on macOS and Linux systems.

This document outlines the core architecture, execution lifecycle, design decisions, and implementation plan.

## Core Concepts

* **Snippet Identifier:** `owner/name.<type>@reference` (e.g., `snippets/hello.sh@v1`, `owner/task.js@a1b2c3d`)
* **Immutable Type:** The repository suffix is authoritative metadata: `.sh` selects Bash, `.js` selects Node.js, and `.py` selects Python. A snippet cannot change type after creation.
* **Registry-Driven Git:** The server handles Git operations. The CLI communicates via HTTP to resolve references and download point-in-time archives, avoiding local `git clone`.
* **Go CLI Runner:** Written in Go for fast native execution (`syscall.Exec`), robust concurrency, and single-binary distribution on macOS/Linux without requiring users to install a runtime just for the CLI. Windows is explicitly unsupported and rejected at startup.

## Architecture & Roadmap

Development proceeds in three phases: **CLI first**, then **registry server improvements**, finally **website** to integrate both.

### Phase 1 - Go CLI
- `run` binary, built via `cmd/run/`, distributed by private CI for darwin/arm64+amd64 and linux/arm64+amd64.
- Installation: curl|sh installer at `install.snippets.run`. Repointing the current legacy installer is release work still to do.

### Phase 2 - Registry Server
- The git-backed registry implements `/api/resolve` and `/api/download` and is deployed at `registry.snippets.run`.
- Its source and Docker image are maintained in the separate `snippets-run/registry` repository.

Repositories are available to the registry as `$SNIPPET_REPOSITORIES_PATH/<owner>/<name>.<type>` and may be bare or working-tree repositories.

### Phase 3 - Web App
- `snippets.run` (Vue SPA) updated to call both the new registry API and the CLI runner interface. Legacy `/index`, `/search`, `/snippets/...` must survive long enough for migration.

## The Execution Lifecycle

The CLI processes snippet execution in three sequential phases:

### 1. Resolution Phase

When a user runs `run owner/hello.sh@v1`:
- The CLI performs an HTTP GET to `/api/resolve/{owner}/{repo}@{ref}` (where `{ref}` may be a branch, tag, or full commit hash).
- The server resolves the ref to a specific Git commit hash and returns the authoritative snippet type:
  ```json
  {
    "owner": "octocat",
    "repo": "hello-world.sh",
    "type": "bash",
    "ref": "v1",
    "commit": "a1b2c3d4"
  }
  ```
- If the user passes `--offline` (future work), bypass network and look up a previously cached ref→commit index at `$SNIPPET_CACHE_PATH/{owner}/{repo}/refs.json`. **Offline mode is not yet implemented.** In v1, all resolution goes through the registry server.

**API spec: download endpoint**:
- `GET /api/download/{owner}/{repo}@{commit}` returns a `.tar.gz` binary stream of the commit's tree.
- Content-Type: `application/gzip`.
  - The client supports tarballs only; if the server returns `.zip`, it must be rejected with an error.

### 2. Prepare Phase (Blocking)

Before running code, the CLI ensures the snippet and its dependencies are on disk.

* **Cache Check:** If `$SNIPPET_CACHE_PATH/{owner}/{repo}/{commit}/` exists and is non-empty, skip download entirely.
* **Atomic Download + Extraction:** Streams the `.tar.gz` into `{owner}/{repo}/{commit}.tmp.{pid}`, then performs an atomic `os.Rename` to the final directory. No partial states leak if interrupted mid-extraction.

### 3. Run Phase (Process Replacement)

Once prepped, the CLI executes the snippet:
* **Input resolution:** All user flags (`--key=value`) are injected as `INPUTS_KEY` environment variables under the process context of the runtime binary. No text-based template replacement — inputs go strictly via env vars to prevent code injection.
* **Process replacement:** The Go CLI uses `syscall.Exec` (not `os/exec` child processes) entirely replacing itself in memory with the target runtime, so standard I/O and signals (`Ctrl+C`) propagate natively without any proxying logic.

## Runtime Scope & Dependency Installation

Supported runtimes per entrypoint:

| Suffix | Runtime | Entrypoint | Dependencies | Notes |
|--------|---------|------------|--------------|-------|
| `.js` | Node.js | exactly one of `index.js` or `index.mjs` | `pnpm install --no-frozen-lockfile` when `package.json` exists | Hard error if `pnpm` is missing |
| `.sh` | Bash | `main.sh` | none | Executed via `bash` |
| `.py` | Python | `main.py` | none | Executed via `python3`; dependency installation is deferred |

Unsupported repository suffixes and mismatched registry types are rejected before execution.

The runner never infers runtime from repository contents. It validates that the registry-provided type matches the repository suffix, then uses the single entrypoint assigned to that type. `package.json` controls Node.js dependency installation only.

## Input Management

* All user-provided flags (`--key=value`) are passed as environment variables with an `INPUTS_` prefix.
  * Example: `run owner/greet.sh@latest --name="Alice"` sets `INPUTS_NAME=Alice`.
* The CLI does not validate, prompt for, or request input declarations from the author — it simply sets and passes whatever flags are received. Prompting/validation via a snippet manifest is possible in future iterations (see the `snippet.json` section).

## Entrypoint Discovery

Entrypoint discovery follows an **explicit-only** model with no arbitrary single-file fallback:

| Runtime | Entrypoint | Execution Command |
|---------|------------|-------------------|
| Node.js | exactly one of `index.js` or `index.mjs` | `node <entrypoint>` |
| Python | `main.py` | `python3 main.py` |
| Bash | `main.sh` | `bash main.sh` |

Files for other runtimes are ignored. Bash and Python have one required entrypoint. Node accepts either `index.js` or `index.mjs`, but rejects a repository containing both.

## Local Cache Management

* **Cache Path:** Configured entirely via `$SNIPPET_CACHE_PATH` environment variable with no built-in default (if unset, the CLI errors out clearly). Users set it explicitly before invoking `run`.
* **Structure:** All snippets are saved under `{cache}/<owner>/<repo>/<commit>/`. A `refs.json` index is planned for the future offline implementation.
* Reserved root-level commands:
  * `run cache status` → prints total disk space used by `$SNIPPET_CACHE_PATH`. No flags; always human-readable.
  * `run cache clean` → deletes `$SNIPPET_CACHE_PATH` + its contents entirely (the pnpm global store is unaffected).

## API Reference

### Request contract — resolve endpoint

```
GET /api/resolve/{owner}/{repo}@{ref}
Accept: application/json

Response 200:
{
    "owner": "octocat",
    "repo": "hello-world.sh",
    "type": "bash",
    "ref": "v1",
    "commit": "a1b2c3d"
}

Response 404:  Not found
Response 4xx/5xx: standard HTTP error codes.
```

**Server must return valid JSON in case of errors (e.g. `{"error":"not found"}`). The Go client should parse and surface this on stderr.**

### Request contract — download endpoint

```
GET /api/download/{owner}/{repo}@{commit}
Accept-encoding: identity, gzip
Content-Type: application/octet-stream | application/gzip

Response 200: binary tarball/stream of the commit's tree. .tar.gz only.
```

### Client-side HTTP details
- TLS via Go stdlib only — no custom CAs in v1; standard system trust store used as-is.
- User-Agent header: `run/<version>` (server can use for analytics).
- Timeout: 10-second resolution phase / 120-second total download phase.
- No retries in v1. Fail on first attempt.
- Redirect handling: Go's standard HTTP client follows redirects with its default 10-hop limit.

## Input Management Rules

* All user-provided flags (`--key=value`) are passed as env vars via an `INPUTS_` prefix — **text-based template replacement is strictly forbidden** (no `${{name}}`, no Jinja/Nunjucks interpolation).
- Example: `--name="Alice"` → `env.{"INPUTS_NAME": "Alice"}` only.

## Error Model

* Exit codes: 0 = success, 1 = runtime/registry error, 2 = usage/arg errors (missing ref, unsupported OS, etc.). Help exits 0 on explicit invocation with `--help` / `help`; invoking without arguments exits 2.
* Errors written as one plain-text line to stderr: e.g., `error: snippet not found: invalid owner/repo format`. No JSON or verbose trace output on failure in v1.

## Build & Distribution

- **Binary name:** `run` (binary installed as `go install github.com/snippets-run/runners/cmd/run@latest`).
- **Module path:** `github.com/snippets-run/runners`.
- **CI/Release:** Every push to `main` triggers `/data/cloud/workflows/on/runners-release.yaml`. It tests the runner, cross-compiles darwin/arm64+amd64 and linux/arm64+amd64 static binaries, generates SHA-256 checksums, and publishes a commit-addressed GitHub release as the latest release.
- **Installer:** `install.sh` downloads the matching GitHub Release asset, validates its SHA-256 checksum, and installs `run` to `$HOME/.local/bin` (or `$INSTALL_DIR`). `install.snippets.run` will be repointed from the legacy bash one-liner after the first release exists.
- Go toolchain: 1.24+ required in CI + local development (pin a current stable, e.g., **1.24.x**). No runtime version requirement beyond 1.22 for stdlib compatibility.

## Repository Layout

Clean break from the legacy Node.js / Bash runners (`bash/`, `node/` directories are deleted). Go code lives under:
```
runners/
├── README.md                 # this file + spec
├── cmd/run/                  # Main binary entrypoint (CLI parsing and CLI execution)
│   └── main.go               # Main() bootstrap, dispatches via the registry & runner packages
├── internal/                 # Internal non-exported packages (all runtime logic)
│   ├── discover/             # Suffix validation and entrypoint selection
│   ├── cache/                # Cache paths, status, and cleaning
│   ├── extract/              # Safe tar.gz extraction
│   ├── registry/             # Resolve/download HTTP client
│   └── run/                  # Dependency preparation + syscall.Exec
├── .goreleaser.yaml          # goreleaser config (cross-build + publish to GitHub Releases)
└── go.mod                    # Go module definition
```

## Future Iteration Work

- **Private snippets & Authentication:** Introduce `run auth login` — store Personal Access Tokens in the OS keychain for private repo tarball access.
- **Offline Mode:** Fully implement `--offline` flag to skip network resolution and run strictly from the local `$SNIPPET_CACHE_PATH`. Ref-indexes map ref → commit.
- **Input Manifest (`snippet.json`):** Move snippet-declared inputs (names, descriptions) into a `snippet.json` front-matter or manifest file so the CLI can prompt interactively like the legacy Node runner — if and when authors want to declare their snippet's required values ahead-of-time.
