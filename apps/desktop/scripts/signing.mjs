import { execFileSync } from "node:child_process";
import { appendFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";

const requireSecret = name => {
  if (!process.env[name]) throw new Error(`Release signing requires ${name}`);
  return process.env[name];
};
const exportEnv = (name, value) => appendFileSync(requireSecret("GITHUB_ENV"), `${name}=${value}\n`);
const temporary = requireSecret("RUNNER_TEMP");
if (process.platform === "darwin") {
  if (!process.argv.includes("--notary-only")) {
    const certificate = join(temporary, "daybee-certificate.p12");
    writeFileSync(certificate, Buffer.from(requireSecret("MAC_CERT_P12_BASE64"), "base64"), { mode: 0o600 });
    const password = requireSecret("MAC_CERT_P12_PASSWORD");
    const keychain = join(temporary, "daybee-build.keychain");
    const run = args => execFileSync("security", args, { stdio: "pipe" });
    run(["create-keychain", "-p", password, keychain]);
    run(["set-keychain-settings", "-lut", "21600", keychain]);
    run(["unlock-keychain", "-p", password, keychain]);
    run(["import", certificate, "-k", keychain, "-P", password, "-T", "/usr/bin/codesign"]);
    run(["list-keychains", "-d", "user", "-s", keychain]);
    run(["set-key-partition-list", "-S", "apple-tool:,apple:,codesign:", "-s", "-k", password, keychain]);
    const identities = run(["find-identity", "-v", "-p", "codesigning", keychain]).toString();
    const identity = identities.match(/"(Developer ID Application:[^"]+)"/)?.[1];
    if (!identity) throw new Error("A Developer ID Application certificate is required");
    exportEnv("MYGO_MACOS_SIGNING_IDENTITY", identity);
  }
  const key = join(temporary, "AuthKey.p8");
  writeFileSync(key, Buffer.from(requireSecret("APPLE_API_KEY_P8_BASE64"), "base64"), { mode: 0o600 });
  execFileSync("xcrun", ["notarytool", "store-credentials", "daybee-notary", "--key", key,
    "--key-id", requireSecret("APPLE_API_KEY_ID"), "--issuer", requireSecret("APPLE_API_ISSUER")], { stdio: "pipe" });
  exportEnv("MYGO_NOTARY_PROFILE", "daybee-notary");
}
