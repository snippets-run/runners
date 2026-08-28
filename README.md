# Snippets.run CLI Architecture & Specification

Snippets.run is a version-controlled, executable snippet registry backed by Git. The CLI is designed to seamlessly resolve, download, prepare, and execute remote code snippets locally on macOS and Linux systems.

This document outlines the core architecture, execution lifecycle, and design decisions for the CLI runner.

## Core Concepts

* **Snippet Identifier:** Follows the pattern `owner/repo@reference` (e.g., `octocat/hello-world@v1` or `octocat/hello-world@abcd012`).
* **API-Driven Git:** The server handles Git operations. The CLI interacts with HTTP endpoints to resolve references and download point-in-time archives (tarballs), avoiding the overhead of local Git clones.
* **Go-Based CLI:** The runner is built in Go to leverage fast native execution (`syscall.Exec`), robust concurrency, and easy distribution for macOS and Linux without requiring users to install an underlying runtime just for the CLI. Windows is explicitly not supported in this iteration.

## The Execution Lifecycle

The CLI processes snippet execution in three distinct, sequential phases:

### 1. Resolution Phase

When a user runs a snippet (e.g., `run owner/repo@v1`), the CLI must determine the exact code to execute:

* The CLI makes a lightweight API request to `/api/resolve/owner/repo@v1`.
* The server resolves the tag or branch (`v1`) to a specific Git commit hash (e.g., `abcd0123`).
* If the user passes an `--offline` flag, the CLI bypasses the network check and attempts to find a previously cached hash for that tag locally.

### 2. Prepare Phase (Blocking)

Before running the code, the CLI must ensure the snippet and its dependencies are present on disk.

* **Cache Check:** The CLI checks if `$SNIPPET_CACHE_PATH/owner/repo/abcd0123/` exists.
* **Atomic Download:** If missing, it downloads a `.tar.gz` or `.zip` of the commit from the server. To prevent race conditions from concurrent executions, the CLI extracts the archive into a temporary directory (e.g., `abcd0123.tmp.[pid]`) and performs an atomic rename to the final hash folder.
* **Dependency Installation:** The CLI scans for dependency manifests (like `package.json`). If found, it executes the required package manager as a blocking child process. For Node.js snippets, **`pnpm` is strictly required** to leverage its global store and prevent disk bloat. The CLI executes `pnpm install --no-frozen-lockfile` to install dependencies while ignoring committed locks from other package managers.

### 3. Run Phase (Process Replacement)

Once the environment is prepped, the CLI executes the snippet.

* **Input Resolution:** User-provided inputs are injected as environment variables.
* **Process Replacement:** Instead of spawning a child process or writing temporary shell wrappers, the Go CLI uses `syscall.Exec`. This entirely replaces the CLI process in memory with the target runtime (e.g., `node`, `python3`).
* **Signal Inheritance:** Because the CLI process is replaced natively, all standard I/O (stdin, stdout, stderr) and system signals (like `Ctrl+C` / SIGINT) are handled directly by the snippet runtime without requiring complex proxying logic.

## Input Management

To prevent code injection vulnerabilities, text replacement (e.g., swapping `{{input}}` inside code files) is strictly prohibited.

All inputs are passed to the runtime environment natively. To prevent collisions with standard system variables (like `PATH` or `USER`), all inputs are automatically prefixed with `INPUTS_`.

* User runs: `run owner/greet@latest --name="Alice"`
* CLI sets: `INPUTS_NAME="Alice"`
* Snippet code reads: `process.env.INPUTS_NAME` (Node.js) or `os.environ.get('INPUTS_NAME')` (Python).

## Entrypoint Discovery

Since snippets can contain multiple files, the CLI must determine which file to execute. It uses a cascading fallback convention based on the detected runtime:

* **Node.js:** Looks for `index.js`, then `main.js`.
* **Python:** Looks for `main.py`, then `__main__.py`.
* **Shell:** Looks for `main.sh`, then `run.sh`.
* **Single File:** If the snippet repository contains only one executable file, it defaults to that file.

*Future Iteration:* We may introduce a YAML front-matter block or a `snippet.json` manifest to allow authors to explicitly define `entrypoint: "custom-file.js"`.

## Local Cache Management

Because snippets download dependencies and occupy disk space, the CLI reserves specific root-level commands to manage the local environment. When parsing arguments, the CLI checks against reserved keywords before treating the argument as a snippet identifier.

* `run cache status`: Displays the total disk space consumed by `$SNIPPET_CACHE_PATH`.
* `run cache clean`: Evicts all downloaded snippets and tarballs, freeing up disk space. (Note: Since `pnpm` uses a global store, cleaning the snippet cache will remove the symlinks, while the actual packages remain safely managed by `pnpm`).

## Future Roadmap

* **Private Snippets & Authentication:** Introduce `run auth login` to store Personal Access Tokens (PAT) securely in the OS keychain, allowing authorized API access for fetching private repository tarballs.
* **Offline Mode:** Fully implement the `--offline` flag to skip the resolution phase and run strictly from the local cache.
