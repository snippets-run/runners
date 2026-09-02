import { createServer } from "node:http";
import { realpath, stat } from "node:fs/promises";
import { spawn } from "node:child_process";
import { join } from "node:path";

const partPattern = /^[A-Za-z0-9][A-Za-z0-9._-]*$/;
const commitPattern = /^[0-9a-f]{7,64}$/;

export function createRegistryServer({ repositoryRoot }) {
  return createServer(async (request, response) => {
    try {
      if (request.method !== "GET") {
        return sendError(response, 405, "method not allowed");
      }

      const url = new URL(request.url, "http://registry.local");
      if (url.pathname === "/health") {
        return sendJSON(response, 200, { status: "ok" });
      }
      const target = parseTarget(url.pathname);
      if (!target) {
        return sendError(response, 404, "not found");
      }

      const root = await realpath(repositoryRoot);
      const repository = await repositoryPath(root, target.owner, target.repo);
      if (target.kind === "resolve") {
        const commit = await resolveCommit(repository, target.value);
        return sendJSON(response, 200, {
          owner: target.owner,
          repo: target.repo,
          ref: target.value,
          commit,
        });
      }

      if (!commitPattern.test(target.value)) {
        return sendError(response, 400, "invalid commit");
      }
      const commit = await resolveCommit(repository, target.value);
      streamArchive(response, repository, commit);
    } catch (error) {
      if (error.code === "ENOENT" || error.code === "NOT_FOUND") {
        return sendError(response, 404, "snippet or reference not found");
      }
      if (error.code === "INVALID_TARGET") {
        return sendError(response, 400, error.message);
      }
      console.error(error);
      if (!response.headersSent) {
        return sendError(response, 500, "internal server error");
      }
      response.destroy(error);
    }
  });
}

function parseTarget(pathname) {
  for (const [prefix, kind] of [["/api/resolve/", "resolve"], ["/api/download/", "download"]]) {
    if (!pathname.startsWith(prefix)) {
      continue;
    }
    const parts = pathname.slice(prefix.length).split("/");
    if (parts.length !== 2) {
      throw invalidTarget("expected owner/repo@reference");
    }
    const owner = decodePart(parts[0]);
    const [repo, value] = decodePart(parts[1]).split("@", 2);
    if (!partPattern.test(owner) || !partPattern.test(repo) || !value) {
      throw invalidTarget("invalid snippet identifier");
    }
    return { kind, owner, repo, value };
  }
  return null;
}

function decodePart(value) {
  try {
    return decodeURIComponent(value);
  } catch {
    throw invalidTarget("invalid URL encoding");
  }
}

async function repositoryPath(root, owner, repo) {
  const path = join(root, owner, repo);
  const info = await stat(path);
  if (!info.isDirectory()) {
    const error = new Error("not found");
    error.code = "NOT_FOUND";
    throw error;
  }
  return path;
}

async function resolveCommit(repository, ref) {
  const result = await git(repository, ["rev-parse", "--verify", "--end-of-options", `${ref}^{commit}`]);
  if (result.code !== 0) {
    const error = new Error("not found");
    error.code = "NOT_FOUND";
    throw error;
  }
  const commit = result.stdout.trim();
  if (!/^[0-9a-f]{40,64}$/.test(commit)) {
    throw new Error("git returned an invalid commit");
  }
  return commit;
}

function streamArchive(response, repository, commit) {
  const child = spawn("git", ["-C", repository, "archive", "--format=tar.gz", commit], {
    stdio: ["ignore", "pipe", "pipe"],
  });
  let stderr = "";
  child.stderr.on("data", (chunk) => {
    stderr += chunk;
  });
  child.once("error", (error) => {
    if (!response.headersSent) {
      sendError(response, 500, error.message);
    } else {
      response.destroy(error);
    }
  });
  child.once("close", (code) => {
    if (code !== 0 && !response.writableEnded) {
      console.error(`git archive failed: ${stderr.trim()}`);
      response.destroy();
    }
  });
  response.writeHead(200, {
    "content-type": "application/gzip",
    "cache-control": "public, immutable, max-age=31536000",
  });
  child.stdout.pipe(response);
}

function git(repository, arguments_) {
  return new Promise((resolve, reject) => {
    const child = spawn("git", ["-C", repository, ...arguments_], { stdio: ["ignore", "pipe", "pipe"] });
    let stdout = "";
    child.stdout.on("data", (chunk) => {
      stdout += chunk;
    });
    child.once("error", reject);
    child.once("close", (code) => resolve({ code, stdout }));
  });
}

function invalidTarget(message) {
  const error = new Error(message);
  error.code = "INVALID_TARGET";
  return error;
}

function sendJSON(response, status, value) {
  const body = JSON.stringify(value);
  response.writeHead(status, { "content-type": "application/json" });
  response.end(body);
}

function sendError(response, status, message) {
  sendJSON(response, status, { error: message });
}

if (import.meta.main) {
  const repositoryRoot = process.env.SNIPPET_REPOSITORIES_PATH;
  if (!repositoryRoot) {
    throw new Error("SNIPPET_REPOSITORIES_PATH is required");
  }
  const port = Number.parseInt(process.env.PORT ?? "3000", 10);
  const server = createRegistryServer({ repositoryRoot });
  server.listen(port, "0.0.0.0", () => {
    console.log(`registry listening on ${port}`);
  });
}
