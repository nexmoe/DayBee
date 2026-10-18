import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { copyFileSync, mkdirSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { verifyUpdate } from "./verify-update.mjs";

const [input = "artifacts", output = "dist"] = process.argv.slice(2).map(path => resolve(path));
const { version, productName } = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
if (process.env.GITHUB_REF_TYPE === "tag") {
  assert.equal(process.env.GITHUB_REF_NAME, `v${version}`, "Release tag does not match package version");
}
mkdirSync(output, {recursive:true});
const hashes = new Map();
const hash = path => createHash("sha256").update(readFileSync(path)).digest("hex");
function collect(directory) {
  for (const file of readdirSync(directory, {withFileTypes:true})) {
    const path=join(directory,file.name);
    if (file.isDirectory()) { if (!file.name.endsWith(".app")) collect(path); continue; }
    if (!/\.dmg$| Setup .*\.exe$|\.tar\.gz$|\.delta$|^update-.*\.json$/.test(file.name)) continue;
    const digest=hash(path);
    assert(!hashes.has(file.name) || hashes.get(file.name)===digest, `Release filename collision: ${file.name}`);
    hashes.set(file.name,digest);copyFileSync(path,join(output,file.name));
  }
}
collect(input);
for (const target of ["darwin-arm64","darwin-amd64","windows-amd64"]) {
  const path=join(output,`update-${target}.json`);
  const manifest=verifyUpdate(output,target);
  assert.equal(manifest.version, version, `Update version does not match package version: ${target}`);
  assert(manifest.signature && manifest.version, `Unsigned update: ${target}`);
  assert(hashes.has(decodeURIComponent(new URL(manifest.url).pathname.split("/").pop())), `Missing update archive: ${target}`);
  const installer = target.startsWith("darwin")
    ? `${productName} ${version}-${target}.dmg` : `${productName} Setup ${version}.exe`;
  assert(hashes.has(installer), `Missing installer: ${installer}`);
}
writeFileSync(join(output,"SHA256SUMS.txt"),[...hashes].sort(([a],[b])=>a.localeCompare(b)).map(([name,digest])=>`${digest}  ${name}`).join("\n")+"\n");
console.log(`Collected ${hashes.size} release files without collisions.`);
