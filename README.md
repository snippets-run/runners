# Snippets.run CLI Architecture & Specification

Snippets.run is a version-controlled, executable snippet registry backed by Git. The CLI resolves, downloads, prepares, and executes remote code snippets locally on macOS and Linux systems.

This document outlines the core architecture, execution lifecycle, design decisions, and implementation plan.

## Core Concepts

* **Snippet Identifier:** `owner/repo@reference` (e.g., `octocat/hello-world@v1`, `owner/repo@a1b2c3d`)
* **Registry-Driven Git:** The server handles Git operations. The CLI communicates via HTTP to resolve references and download point-in-time archives, avoiding local `git clone`.
* **Go CLI Runner:** Written in Go for fast native execution (`syscall.Exec`), robust concurrency, and single-binary distribution on macOS/Linux without requiring users to install a runtime just for the CLI. Windows is explicitly unsupported in this iteration; unsupported OSes are handled by compile-time guards + explicit panics at runtime.

## Architecture & Roadmap

Development proceeds in three phases: **CLI first**, then **registry server improvements**, finally **website** to integrate both.

### Phase 1 - Go CLI (current work)
- `run` binary, built via `cmd/run/`, distributed by goreleaser for darwin/arm64+amd64 and linux/arm64+amd64.
- Installation: curl|sh installer at `install.snippets.run`. Repointing the current legacy installer is release work still to do.

### Phase 2 - Registry Server
- A new git-backed registry implementing `/api/resolve` and `/api/download`.
- The current server (`registry.snippets.run`) uses a legacy single-file KV model (`/s/:platform/:name` with no Git). Both run in parallel during transition until the old endpoints are fully retired.

The implementation is in `server/`. It has no runtime dependencies beyond Node.js and `git`.

```sh
SNIPPET_REPOSITORIES_PATH=/repositories node server/src/server.mjs
```

Repositories must be available as `$SNIPPET_REPOSITORIES_PATH/<owner>/<repo>`, and may be bare repositories or working-tree repositories. The image in `server/Dockerfile` expects the repository root to be mounted read-only at `/repositories`.

### Phase 3 - Web App
- `snippets.run` (Vue SPA) updated to call both the new registry API and the CLI runner interface. Legacy `/index`, `/search`, `/snippets/...` must survive long enough for migration.

## The Execution Lifecycle

The CLI processes snippet execution in three sequential phases:

### 1. Resolution Phase

When a user runs `run owner/repo@v1`:
- The CLI performs an HTTP GET to `/api/resolve/{owner}/{repo}@{ref}` (where `{ref}` may be a branch, tag, or full commit hash).
- The server resolves the ref → a specific Git commit hash. Response:
  ```json
  {
    "owner": "octocat",
    "repo": "hello-world",
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

* **Cache Check:** If `$SNIPPET_CACHE_PATH/{owner}/{repo}/{commit}/` exists, skip download entirely.
* **Atomic Download + Extraction:** Streams the `.tar.gz` into `{owner}/{repo}/{commit}.tmp.{pid}`, then performs an atomic `os.Rename` to the final directory. No partial states leak if interrupted mid-extraction.

### 3. Run Phase (Process Replacement)

Once prepped, the CLI executes the snippet:
* **Input resolution:** All user flags (`--key=value`) are injected as `INPUTS_KEY` environment variables under the process context of the runtime binary. No text-based template replacement — inputs go strictly via env vars to prevent code injection.
* **Process replacement:** The Go CLI uses `syscall.Exec` (not `os/exec` child processes) entirely replacing itself in memory with the target runtime, so standard I/O and signals (`Ctrl+C`) propagate natively without any proxying logic.

## Runtime Scope & Dependency Installation

Supported runtimes per entrypoint:

| Runtime    | Discovery                     | Dependencies                       | Notes                                           |
|------------|-------------------------------|------------------------------------|-------------------------------------------------|
| Node.js    | `package.json`, `.js` files  | `pnpm install --no-frozen-lockfile`| Hard error if `pnpm` is missing from PATH       |
| Shell      | `main.sh`, `run.sh`          | none                               | Executed via `bash`                             |
| Python     | `pyproject.toml`, `.py` files| none (manual pip install for user) | Executed via `python3` only                     |

Unsupported snippets (detected but no handler exists) return a clear error message listing what was found and that support is not yet implemented.

### Dependency Manifest Detection Order
Runtime selection uses root-level manifests where present: `package.json` selects Node.js; `requirements.txt` or `pyproject.toml` selects Python. If more than one runtime has an explicit entrypoint and no manifest selects one, the runner reports an ambiguity error.

## Input Management

* All user-provided flags (`--key=value`) are passed as environment variables with an `INPUTS_` prefix.
  * Example: `run owner/greet@latest --name="Alice"` sets `INPUTS_NAME=Alice`.
* The CLI does not validate, prompt for, or request input declarations from the author — it simply sets and passes whatever flags are received. Prompting/validation via a snippet manifest is possible in future iterations (see the `snippet.json` section).

## Entrypoint Discovery

Since snippets can contain multiple files, entrypoint discovery follows an **explicit-only** model — no implicit "single file" fallback to avoid unpredictable behaviour. The CLI scans for and returns the first found match for each supported runtime in this priority order:

| Runtime | Explicit Entry Points (in order)     | Execution Command              |
|---------|--------------------------------------|-------------------------------|
| Node.js | `index.js`, `main.js`               | `node index.js` / `node main.js`  |
| Python  | `main.py`, `__main__.py`            | `python3 main.py` / `python3 __main__.py` |
| Shell   | `main.sh`, `run.sh`                 | `bash main.sh` / `bash run.sh`      |

When no manifest is present, exactly one supported explicit entrypoint is required. Multiple runtimes are an ambiguity error. The snippet author must explicitly provide one of the expected file names; there is no fallback to arbitrary executable files.

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
    "repo": "hello-world",
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
- Redirect handling: follow redirects by default with a cap of 5 hops.

## Input Management Rules

* All user-provided flags (`--key=value`) are passed as env vars via an `INPUTS_` prefix — **text-based template replacement is strictly forbidden** (no `${{name}}`, no Jinja/Nunjucks interpolation).
- Example: `--name="Alice"` → `env.{"INPUTS_NAME": "Alice"}` only.

## Error Model

* Exit codes: 0 = success, 1 = runtime/registry error, 2 = usage/arg errors (missing ref, unsupported OS, etc.). Help exits 0 on explicit invocation with `--help` / `help`; invoking without arguments exits 2.
* Errors written as one plain-text line to stderr: e.g., `error: snippet not found: invalid owner/repo format`. No JSON or verbose trace output on failure in v1.

## Build & Distribution

- **Binary name:** `run` (binary installed as `go install github.com/snippets-run/runners/cmd/run@latest`).
- **Module path:** `github.com/snippets-run/runners`.
- **CI/Release:** goreleaser for cross-compilation: darwin/arm64+amd64, linux/arm64+amd64. Built as static binaries (`-ldflags "-s -w"`, `CGO_ENABLED=0`).
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
│   ├── discover/             # Entrypoint discovery + manifest detection
│   ├── cache/                # Cache paths, status, and cleaning
│   ├── extract/              # Safe tar.gz extraction
│   ├── registry/             # Resolve/download HTTP client
│   └── run/                  # Dependency preparation + syscall.Exec
├── server/                    # Git-backed Node.js registry and Docker image
├── .goreleaser.yaml          # goreleaser config (cross-build + publish to GitHub Releases)
├── go.mod                    # Go module definition
└── go.sum                    # Module hashes
```

## Future Iteration Work

- **Private snippets & Authentication:** Introduce `run auth login` — store Personal Access Tokens in the OS keychain for private repo tarball access.
- **Offline Mode:** Fully implement `--offline` flag to skip network resolution and run strictly from the local `$SNIPPET_CACHE_PATH`. Ref-indexes map ref → commit.
- **Input Manifest (`snippet.json`):** Move snippet-declared inputs (names, descriptions) into a `snippet.json` front-matter or manifest file so the CLI can prompt interactively like the legacy Node runner — if and when authors want to declare their snippet's required values ahead-of-time.
