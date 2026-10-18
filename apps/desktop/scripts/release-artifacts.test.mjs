import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash, generateKeyPairSync, sign } from "node:crypto";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { verifyReleaseGates } from "./release-gates.mjs";

test("release gates require every architecture, eight measured hours and an evidence file", () => {
  const scratch = mkdtempSync(join(tmpdir(), "daybee-release-gates-"));
  try {
    writeFileSync(join(scratch, "evidence.md"), "Synthetic acceptance fixture");
    const report = { targets: Object.fromEntries(["darwin-arm64", "darwin-amd64", "windows-amd64"].map(target => [target, {
      uiIdentical: true, nativeInference: true, minimumOS: true, cleanInstall: true,
      signedUpdateRecovery: true, enduranceHours: 8, evidence: "evidence.md"
    }])) };
    assert.doesNotThrow(() => verifyReleaseGates(report, scratch));
    const missingTarget = structuredClone(report); delete missingTarget.targets["windows-amd64"];
    assert.throws(() => verifyReleaseGates(missingTarget, scratch), /windows-amd64.uiIdentical/);
    for (const hours of [undefined, null, "8", 0, 7.99, Infinity, NaN]) {
      const missingHours = structuredClone(report); missingHours.targets["darwin-arm64"].enduranceHours = hours;
      assert.throws(() => verifyReleaseGates(missingHours, scratch), /8-hour recording/);
    }
    const missingFile = structuredClone(report); missingFile.targets["darwin-amd64"].evidence = "missing.md";
    assert.throws(() => verifyReleaseGates(missingFile, scratch), /evidence file is missing/);
    const incomplete = structuredClone(report); incomplete.targets["darwin-arm64"].uiIdentical = false;
    assert.throws(() => verifyReleaseGates(incomplete, scratch), /darwin-arm64.uiIdentical/);
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
});

test("release collection verifies three signatures and filename collisions", () => {
  const scratch = mkdtempSync(join(tmpdir(), "daybee-release-artifacts-"));
  const { privateKey, publicKey } = generateKeyPairSync("ed25519");
  const rawKey = publicKey.export({ format: "der", type: "spki" }).subarray(-32).toString("base64");
  const targets = ["darwin-arm64", "darwin-amd64", "windows-amd64"];
  const { version } = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
  const installer = `DayBee Setup ${version}.exe`;
  const input = join(scratch, "input");
  const paths = targets.map(target => join(input, target));
  const manifests = [];
  try {
    targets.forEach((target, i) => {
      const directory = paths[i]; mkdirSync(directory, { recursive: true });
      const archive = `daybee-${version}-${target}.tar.gz`;
      const data = Buffer.from(`fixture archive for ${target}`);
      writeFileSync(join(directory, archive), data);
      if (i === 2) writeFileSync(join(directory, installer), "Windows installer");
      else writeFileSync(join(directory, `DayBee ${version}-${target}.dmg`), `Mac installer for ${target}`);
      const manifest = {
        version, size: data.length,
        url: `https://example.com/${archive}`,
        signature: sign(null, createHash("sha256").update(data).digest(), privateKey).toString("base64")
      };
      manifests.push(manifest);
      writeFileSync(join(directory, `update-${target}.json`), JSON.stringify(manifest));
    });
    const collect = (output, env = {}) => spawnSync(process.execPath,
      [join(import.meta.dirname, "collect-release.mjs"), input, join(scratch, output)],
      { encoding: "utf8", env: { ...process.env, DAYBEE_UPDATER_PUBLIC_KEY: rawKey,
        GITHUB_REF_TYPE: "tag", GITHUB_REF_NAME: `v${version}`, ...env } });
    const valid = collect("valid");
    assert.equal(valid.status, 0, valid.stderr);
    assert.equal(readFileSync(join(scratch, "valid", "SHA256SUMS.txt"), "utf8").trim().split("\n").length, 9);

    const wrongTag = collect("wrong-tag", { GITHUB_REF_NAME: "v0.0.0" });
    assert.notEqual(wrongTag.status, 0);
    assert.match(wrongTag.stderr, /Release tag does not match package version/);

    const macInstaller = join(paths[0], `DayBee ${version}-${targets[0]}.dmg`);
    rmSync(macInstaller);
    const missingInstaller = collect("missing-installer");
    assert.notEqual(missingInstaller.status, 0);
    assert.match(missingInstaller.stderr, /Missing installer/);
    writeFileSync(macInstaller, "Restored Mac installer");

    writeFileSync(join(paths[1], installer), "different installer");
    const collision = collect("collision");
    assert.notEqual(collision.status, 0);
    assert.match(collision.stderr, /Release filename collision/);
    rmSync(join(paths[1], installer));

    const manifestPath = join(paths[0], `update-${targets[0]}.json`);
    writeFileSync(manifestPath, JSON.stringify({ ...manifests[0], version: "0.0.0" }));
    const wrongVersion = collect("wrong-version");
    assert.notEqual(wrongVersion.status, 0);
    assert.match(wrongVersion.stderr, /Update version does not match package version/);
    writeFileSync(manifestPath, JSON.stringify({ ...manifests[0], signature: Buffer.alloc(64).toString("base64") }));
    const invalidSignature = collect("invalid-signature");
    assert.notEqual(invalidSignature.status, 0);
    assert.match(invalidSignature.stderr, /Update signature mismatch/);
    writeFileSync(manifestPath, JSON.stringify({ ...manifests[0], size: manifests[0].size + 1 }));
    const invalidSize = collect("invalid-size");
    assert.notEqual(invalidSize.status, 0);
    assert.match(invalidSize.stderr, /Update archive size mismatch/);
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
});
