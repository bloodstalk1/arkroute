#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);

export function platformPackageName(platform = process.platform, arch = process.arch) {
  const key = `${platform}-${arch}`;
  switch (key) {
    case "darwin-arm64":
      return "@arkroute/darwin-arm64";
    case "darwin-x64":
      return "@arkroute/darwin-x64";
    case "linux-arm64":
      return "@arkroute/linux-arm64";
    case "linux-x64":
      return "@arkroute/linux-x64";
    case "win32-x64":
      return "@arkroute/win32-x64";
    default:
      throw new Error(`unsupported platform ${key}`);
  }
}

export function binaryName(platform = process.platform) {
  return platform === "win32" ? "arkroute.exe" : "arkroute";
}

export function resolveBinary(platform = process.platform, arch = process.arch) {
  const packageName = platformPackageName(platform, arch);
  const packageJSON = require.resolve(`${packageName}/package.json`);
  return path.join(path.dirname(packageJSON), "bin", binaryName(platform));
}

const __filename = fileURLToPath(import.meta.url);
if (__filename === path.resolve(process.argv[1])) {
  let binary;
  try {
    binary = resolveBinary();
  } catch (error) {
    console.error(`Arkroute binary for ${process.platform}-${process.arch} is not installed.`);
    console.error("Try reinstalling with optional dependencies enabled:");
    console.error("  npm install -g arkroute");
    process.exit(1);
  }
  const result = spawnSync(binary, process.argv.slice(2), { stdio: "inherit" });
  if (result.error) {
    console.error(result.error.message);
    process.exit(1);
  }
  process.exit(result.status ?? 0);
}
