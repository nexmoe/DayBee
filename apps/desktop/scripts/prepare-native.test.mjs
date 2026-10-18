import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { prepareMacLibrary, prependNativePath } from "./prepare-native.mjs";

test("preserves the Windows Path key and existing executable search directories", () => {
  const env = { Path: "C:\\Go\\bin;C:\\Windows\\System32", OTHER: "retained" };
  prependNativePath(env, "C:\\native", ";");
  assert.deepEqual(env, {
    Path: "C:\\native;C:\\Go\\bin;C:\\Windows\\System32", OTHER: "retained"
  });
});

test("avoids duplicate case-insensitive PATH keys", () => {
  const env = { Path: "ignored", PATH: "effective" };
  prependNativePath(env, "native", ";");
  assert.deepEqual(env, { PATH: "native;effective" });
});

test("does not append an undefined path when no path is present", () => {
  const env = {};
  prependNativePath(env, "native", ";");
  assert.deepEqual(env, { PATH: "native" });
});

function libraryFixture(t) {
  const directory = mkdtempSync(join(tmpdir(), "daybee-native-library-"));
  t.after(() => rmSync(directory, { recursive: true, force: true }));
  const source = join(directory, "upstream.dylib");
  const bundled = join(directory, "bundled.dylib");
  const original = Buffer.from([0xcf, 0xfa, 0xed, 0xfe, 0, 1, 2, 3]);
  writeFileSync(source, original);
  return { source, bundled, original };
}

test("restores the upstream library if an older strip corrupts its copy before failing", t => {
  const { source, bundled, original } = libraryFixture(t);
  const calls = [], warnings = [];
  const run = (command, args) => {
    calls.push([command, args]);
    if (command === "xcrun") {
      writeFileSync(bundled, "partially rewritten library");
      throw new Error("link edit information does not fill the __LINKEDIT segment");
    }
  };
  assert.doesNotThrow(() => prepareMacLibrary(source, bundled, {
    run, warn: message => warnings.push(message)
  }));
  assert.deepEqual(readFileSync(bundled), original);
  assert.deepEqual(readFileSync(source), original);
  if (process.platform !== "win32") assert.equal(statSync(bundled).mode & 0o777, 0o755);
  assert.equal(calls.filter(([command]) => command === "codesign").length, 2);
  assert.equal(warnings.length, 1);
  assert.match(warnings[0], /__LINKEDIT/);
});

test("keeps a successfully stripped library and leaves the upstream cache intact", t => {
  const { source, bundled, original } = libraryFixture(t);
  const stripped = Buffer.from("smaller library");
  prepareMacLibrary(source, bundled, {
    run: command => { if (command === "xcrun") writeFileSync(bundled, stripped); },
    warn: () => assert.fail("successful stripping must not warn")
  });
  assert.deepEqual(readFileSync(bundled), stripped);
  assert.deepEqual(readFileSync(source), original);
});

test("signature removal failures remain fatal", t => {
  const { source, bundled } = libraryFixture(t);
  assert.throws(() => prepareMacLibrary(source, bundled, {
    run: () => { throw new Error("codesign unavailable"); },
    warn: () => assert.fail("signature errors must not be hidden")
  }), /codesign unavailable/);
});
