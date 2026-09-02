import assert from "node:assert/strict";
import test from "node:test";

import { createSnippetRepository, startRegistry } from "./helpers.mjs";

test("resolves a tag and streams its archive", async (t) => {
  const snippet = await createSnippetRepository();
  const registry = await startRegistry(snippet.root);
  t.after(async () => {
    await registry.close();
    await snippet.remove();
  });

  const resolveResponse = await fetch(`${registry.url}/api/resolve/acme/hello@v1`);
  assert.equal(resolveResponse.status, 200);
  assert.deepEqual(await resolveResponse.json(), {
    owner: "acme",
    repo: "hello",
    ref: "v1",
    commit: snippet.commit,
  });

  const branchStyleRef = await fetch(`${registry.url}/api/resolve/acme/hello@release%2Fv1`);
  assert.equal(branchStyleRef.status, 200);
  assert.equal((await branchStyleRef.json()).ref, "release/v1");

  const archiveResponse = await fetch(`${registry.url}/api/download/acme/hello@${snippet.commit}`);
  assert.equal(archiveResponse.status, 200);
  assert.equal(archiveResponse.headers.get("content-type"), "application/gzip");
  assert.ok((await archiveResponse.arrayBuffer()).byteLength > 0);
});

test("returns JSON errors for missing snippets", async (t) => {
  const snippet = await createSnippetRepository();
  const registry = await startRegistry(snippet.root);
  t.after(async () => {
    await registry.close();
    await snippet.remove();
  });

  const response = await fetch(`${registry.url}/api/resolve/acme/missing@v1`);
  assert.equal(response.status, 404);
  assert.deepEqual(await response.json(), { error: "snippet or reference not found" });
});
