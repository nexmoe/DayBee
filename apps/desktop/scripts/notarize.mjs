import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { fileURLToPath } from "node:url";

export async function notarizeImage(path, profile, { run = execFileSync, wait = delay, warn = console.warn } = {}) {
  const output = run("xcrun", ["notarytool", "submit", path, "--keychain-profile", profile,
    "--wait", "--output-format", "json"], { encoding: "utf8", stdio: ["ignore", "pipe", "inherit"] });
  const result = JSON.parse(output.slice(output.lastIndexOf("{")));
  if (result.status !== "Accepted") throw new Error(`Notarization rejected: ${result.status} (${result.id})`);
  for (const action of ["staple", "validate"]) {
    for (let attempt = 0; ; attempt++) {
      try {
        run("xcrun", ["stapler", action, path], { stdio: "inherit" });
        break;
      } catch (error) {
        const retryable = action === "staple" ? [65, 68] : [68];
        if (!retryable.includes(error.status) || attempt >= 3) throw error;
        const pause = 5000 * 2 ** attempt;
        warn(`Apple ticket ${action} failed (${error.status}); retrying in ${pause / 1000}s.`);
        await wait(pause);
      }
    }
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [directory, target] = process.argv.slice(2);
  const profile = process.env.MYGO_NOTARY_PROFILE;
  if (!directory || !["darwin-arm64", "darwin-amd64"].includes(target) || !profile) {
    throw new Error("Notarization requires an artifact directory, a Mac target and MYGO_NOTARY_PROFILE");
  }
  const pkg = JSON.parse(readFileSync(new URL("../package.json", import.meta.url), "utf8"));
  await notarizeImage(join(resolve(directory), `${pkg.productName} ${pkg.version}-${target}.dmg`), profile);
}
