#!/usr/bin/env node

import { createInterface } from "node:readline/promises";
import { createHash } from "node:crypto";
import { writeFile } from "node:fs/promises";
import { spawn } from "node:child_process";

const templateRe = /\$\{([\s\S]+?)\}/g;
const rl = createInterface({
  input: process.stdin,
  output: process.stdout,
});

async function getSnippet(name) {
  const response = await fetch("https://registry.snippets.run/s/node/" + name);
  if (!response.ok || response.status > 299) {
    throw new Error(response.statusText);
  }

  return response.json();
}

async function getInputs(inputs) {
  if (!inputs || !inputs.length) {
    return {};
  }

  const values = new Map();
  for (const input of inputs) {
    values[input.name] = await rl.question(
      `${input.description || input.name}:\n> `
    );
  }

  return values;
}

function showHelp() {
  console.log(`
How to use:
  run                     Shows this message
  run <snippet-name>      Download and run <snippet-name>
`);
  process.exit(1);
}

(async () => {
  const name = process.argv[2];

  if (!name) {
    showHelp();
  }

  try {
    const { script, inputs } = await getSnippet(name);
    const replacements = await getInputs(inputs);
    const code = script.replace(
      templateRe,
      (_, input) => replacements.get(input.trim()) || ""
    );
    const nodePath = process.argv[0];
    const hash = createHash("sha256").update(name).digest("hex");
    const tmpDir = process.env.TMPDIR || "/tmp";
    const filePath = `${tmpDir}/${hash + ".js"}`;

    await writeFile(filePath, code);
    const p = spawn(nodePath, [filePath], { stdio: "inherit" });
    p.on("exit", (c) => process.exit(c));
  } catch (error) {
    process.stderr.write(String(error));
    process.exit(1);
  }
})();
