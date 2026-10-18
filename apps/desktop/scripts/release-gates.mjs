import { appendFileSync, existsSync, readFileSync, statSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../../../", import.meta.url));
export function verifyReleaseGates(report, evidenceRoot = root) {
  for (const target of ["darwin-arm64", "darwin-amd64", "windows-amd64"]) {
    const result = report.targets?.[target];
    for (const gate of ["uiIdentical", "nativeInference", "minimumOS", "cleanInstall", "signedUpdateRecovery"]) {
      if (result?.[gate] !== true) throw new Error(`Release blocked: ${target}.${gate} has not passed.`);
    }
    if (!Number.isFinite(result.enduranceHours) || result.enduranceHours < 8) {
      throw new Error(`Release blocked: ${target} needs an 8-hour recording acceptance run.`);
    }
    if (typeof result.evidence !== "string" || !result.evidence.trim()) {
      throw new Error(`Release blocked: ${target} needs recorded acceptance evidence.`);
    }
    const path = resolve(evidenceRoot, result.evidence);
    if (!existsSync(path) || !statSync(path).isFile()) {
      throw new Error(`Release blocked: ${target} acceptance evidence file is missing.`);
    }
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const report = JSON.parse(readFileSync(new URL("../../../docs/migration-verification.json", import.meta.url), "utf8"));
  try {
    verifyReleaseGates(report);
    console.log("All migration release gates passed.");
  } catch (error) {
    if (!process.argv.includes("--report")) throw error;
    const message = `${error.message}\nManual migration acceptance remains incomplete; see docs/migration-verification.md.\nPublication requires all automated native checks, complete installers and verified signed updates.\nWindows installers do not use Authenticode signing.\n`;
    console.warn(message);
    if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, message);
  }
}
