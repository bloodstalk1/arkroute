import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

const __dirname = dirname(fileURLToPath(import.meta.url));
const npmDir = join(__dirname, "..", "..");
const repoRoot = join(npmDir, "..");
const repositoryUrl = "https://github.com/bloodstalk1/arkroute";

const publishedPackageJsonPaths = [
  join(npmDir, "arkroute", "package.json"),
  join(npmDir, "platform", "darwin-arm64", "package.json"),
  join(npmDir, "platform", "darwin-x64", "package.json"),
  join(npmDir, "platform", "linux-arm64", "package.json"),
  join(npmDir, "platform", "linux-x64", "package.json"),
  join(npmDir, "platform", "win32-x64", "package.json"),
];

test("published npm packages declare the provenance repository", () => {
  for (const packageJsonPath of publishedPackageJsonPaths) {
    const pkg = JSON.parse(readFileSync(packageJsonPath, "utf8"));

    assert.deepEqual(
      pkg.repository,
      { type: "git", url: repositoryUrl },
      `${pkg.name} must match the GitHub Actions provenance repository`,
    );
  }
});

test("publish workflow uses explicit relative local package paths", () => {
  const workflow = readFileSync(join(repoRoot, ".github", "workflows", "publish.yml"), "utf8");

  assert.doesNotMatch(
    workflow,
    /\bnpm publish npm\//,
    "npm publish treats npm/... as a package spec on GitHub Actions; use ./npm/...",
  );
});

test("publish workflow skips package versions that already exist", () => {
  const workflow = readFileSync(join(repoRoot, ".github", "workflows", "publish.yml"), "utf8");

  assert.match(
    workflow,
    /npm view "\$package@\$\{\s*VERSION\s*\}"/,
    "workflow must check npm before publishing so reruns do not fail on already-published versions",
  );
});
