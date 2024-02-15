#!/usr/bin/env node

import { createInterface } from "node:readline/promises";
import { createHash } from "node:crypto";
import { writeFile } from "node:fs/promises";
import { spawn } from "node:child_process";

const hasPipedInput = !process.stdin.isTTY;

function parseArgs() {
  const argv = process.argv.slice(3);
  const options = {};
  const args = [];
  let flag = "";

  for (const next of argv) {
    if (next.startsWith("--")) {
      if (flag) {
        options[flag] = true;
      }

      flag = next.slice(2);
      continue;
    }

    if (flag) {
      options[flag] = next;
      flag = "";
    } else {
      args.push(next);
    }
  }

  return { args, options };
}

async function getSnippet(name) {
  const response = await fetch("https://registry.snippets.run/s/node/" + name);
  if (!response.ok || response.status > 299) {
    throw new Error(response.statusText);
  }

  return response.json();
}

async function getInputs(inputs, cliOptions) {
  const values = new Map();

  if (!inputs?.length) {
    return values;
  }

  const rl =
    !hasPipedInput &&
    createInterface({
      input: process.stdin,
      output: process.stdout,
    });

  for (const input of inputs) {
    const value =
      cliOptions[input.name] ||
      (hasPipedInput
        ? ""
        : await rl.question(`${input.description || input.name}:\n> `));

    values.set(input.name, value);
  }

  return values;
}

function showHelp() {
  console.log(`
How to use:
  run                     Shows this message
  run <snippet-name>      Download and run <snippet-name>
`);
}

function getCode(inputs, script, replacements) {
  const keys = inputs.map((i) => i.name).join("|");
  const matcher = new RegExp("\\$\\{\\s*?(" + keys + ")\\s*?\\}", "g");
  return script.replace(matcher, (_, input) => replacements.get(input.trim()));
}

async function writeScript(name, code) {
  const hash = createHash("sha256").update(name).digest("hex");
  const tmpDir = process.env.TMPDIR || "/tmp";
  const filePath = `${tmpDir}/${hash + ".mjs"}`;
  await writeFile(filePath, code);
  return filePath;
}

function runScript(filePath, args) {
  const nodePath = process.argv[0];
  const env = {
    SNIPPETS_REGISTRY: "https://registry.snippets.run",
    ...process.env,
  };

  const p = spawn(nodePath, [filePath, ...args], { stdio: "inherit", env });
  p.on("exit", (c) => process.exit(c));
}

(async () => {
  const { args, options } = parseArgs();
  const name = process.argv[2];

  if (!name) {
    showHelp();
    process.exit(1);
  }

  try {
    const { script, inputs } = await getSnippet(name);
    const replacements = await getInputs(inputs, options);
    const code = getCode(inputs, script, replacements);
    const filePath = await writeScript(name, code);

    runScript(filePath, args);
  } catch (error) {
    process.stderr.write(String(error));
    process.exit(1);
  }
})();
