import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import test from "node:test";

import { createSnippetRepository, run, startRegistry } from "./helpers.mjs";

test("Go runner resolves, caches, and executes a shell snippet", async (t) => {
  const root = resolve(import.meta.dirname, "../..");
  const temporary = await mkdtemp(join(tmpdir(), "snippets-run-e2e-"));
  const snippet = await createSnippetRepository();
  const registry = await startRegistry(snippet.root);
  t.after(async () => {
    await registry.close();
    await snippet.remove();
    await rm(temporary, { recursive: true, force: true });
  });

  const binary = join(temporary, "run");
  const build = await run("go", ["build", "-o", binary, "./cmd/run"], { cwd: root });
  assert.equal(build.code, 0, build.stderr);

  const result = await run(binary, ["acme/hello@v1", "--name=Alice"], {
    cwd: root,
    env: {
      ...process.env,
      SNIPPET_CACHE_PATH: join(temporary, "cache"),
      SNIPPET_REGISTRY_URL: registry.url,
    },
  });
  assert.equal(result.code, 0, result.stderr);
  assert.equal(result.stdout, "Alice\n");
});
