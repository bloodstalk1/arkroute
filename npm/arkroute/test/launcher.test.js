import assert from "node:assert/strict";
import test from "node:test";
import { platformPackageName, binaryName, resolveBinary } from "../bin/arkroute.js";

test("platformPackageName resolves supported packages", () => {
  assert.equal(platformPackageName("darwin", "arm64"), "@arkroute/darwin-arm64");
  assert.equal(platformPackageName("linux", "x64"), "@arkroute/linux-x64");
  assert.equal(platformPackageName("win32", "x64"), "@arkroute/win32-x64");
});

test("platformPackageName rejects unsupported targets", () => {
  assert.throws(() => platformPackageName("freebsd", "x64"), /unsupported platform/);
});

test("binaryName appends exe on windows", () => {
  assert.equal(binaryName("win32"), "arkroute.exe");
  assert.equal(binaryName("linux"), "arkroute");
});

test("exports are intact after entrypoint refactor", () => {
  assert.equal(typeof platformPackageName, "function");
  assert.equal(typeof binaryName, "function");
  assert.equal(typeof resolveBinary, "function");
});

test("platformPackageName handles all supported platforms including win32", () => {
  // Ensures the win32 platform (which uses backslash paths with spaces)
  // is still a supported target after the portability fix
  const winPkg = platformPackageName("win32", "x64");
  assert.equal(winPkg, "@arkroute/win32-x64");
  assert.equal(binaryName("win32"), "arkroute.exe");

  // Verify all platforms round-trip correctly
  const platforms = [
    ["darwin", "arm64"], ["darwin", "x64"],
    ["linux", "arm64"], ["linux", "x64"],
    ["win32", "x64"],
  ];
  for (const [plat, arch] of platforms) {
    const name = platformPackageName(plat, arch);
    assert.match(name, /^@arkroute\//, `expected scoped package for ${plat}-${arch}`);
  }
});
