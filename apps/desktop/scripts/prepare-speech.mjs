import { execFileSync } from "node:child_process";
import { mkdirSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

export function prepareSpeech(root, target, env) {
  const directory = join(root, "resources", target, "lib");
  mkdirSync(directory, { recursive: true });
  const source = join(root, "internal", "speechanalyzer", "native");
  const library = join(directory, "libdaybee-speech.dylib");
  const files = readdirSync(source).filter(name => name.endsWith(".swift")).map(name => join(source, name));
  const built = (() => { try { return statSync(library).mtimeMs; } catch { return 0; } })();
  if (files.some(path => statSync(path).mtimeMs > built) || statSync(import.meta.filename).mtimeMs > built) {
    const arch = target.endsWith("arm64") ? "arm64" : "x86_64";
    execFileSync("xcrun", ["swiftc", "-emit-library", "-O", "-target", `${arch}-apple-macosx13.3`,
      "-module-cache-path", join(root, ".generated", "swift-module-cache"),
      "-Xlinker", "-install_name", "-Xlinker", "@rpath/libdaybee-speech.dylib", ...files, "-o", library],
    { cwd: root, env, stdio: "inherit" });
  }
  env.DAYBEE_SPEECH_LIBRARY = library;
}
