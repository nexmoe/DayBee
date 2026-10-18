import { execFileSync } from "node:child_process";
import { setTimeout as delay } from "node:timers/promises";

export async function notarizeImage(path, profile, { run = execFileSync, wait = delay, warn = console.warn } = {}) {
  const output = run("xcrun", ["notarytool", "submit", path, "--keychain-profile", profile,
    "--wait", "--output-format", "json"], { encoding: "utf8", stdio: ["ignore", "pipe", "inherit"] });
  const result = JSON.parse(output.slice(output.lastIndexOf("{")));
  if (result.status !== "Accepted") throw new Error(`Notarization rejected: ${result.status} (${result.id})`);
  for (let attempt = 0; ; attempt++) {
    try {
      run("xcrun", ["stapler", "staple", path], { stdio: "inherit" });
      break;
    } catch (error) {
      if (![65, 68].includes(error.status) || attempt >= 3) throw error;
      const pause = 5000 * 2 ** attempt;
      warn(`Apple ticket lookup failed (${error.status}); retrying stapling in ${pause / 1000}s.`);
      await wait(pause);
    }
  }
  run("xcrun", ["stapler", "validate", path], { stdio: "inherit" });
}
