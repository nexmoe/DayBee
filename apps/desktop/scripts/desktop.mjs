import { spawnSync } from "node:child_process";
import { resolve, join } from "node:path";
import { readdirSync, renameSync, rmSync } from "node:fs";
import { prepareNative } from "./prepare-native.mjs";
import { binaryPath } from "mygo-cli";

const root = resolve(import.meta.dirname, "..");
const [command = "dev", ...args] = process.argv.slice(2);
if (command === "dev" && (!process.env.PORTLESS_URL || !process.env.PORT)) {
  throw new Error("Desktop development requires portless. Run bun run dev from the workspace root or apps/desktop.");
}
const platform = process.platform === "win32" ? "windows" : process.platform;
const arch = process.arch === "x64" ? "amd64" : process.arch;
const target = process.env.DAYBEE_BUILD_TARGET || `${platform}-${arch}`;
if (target !== `${platform}-${arch}`) throw new Error(`Use a native ${target} runner with its C compiler; cross builds are unsupported.`);
const env = { ...process.env, CGO_ENABLED: "1", GOFLAGS: "-mod=mod" };
if (platform === "darwin") {
  env.MACOSX_DEPLOYMENT_TARGET = "13.3";
  env.CGO_CFLAGS = `${env.CGO_CFLAGS || ""} -mmacosx-version-min=13.3`.trim();
  env.CGO_LDFLAGS = `${env.CGO_LDFLAGS || ""} -mmacosx-version-min=13.3`.trim();
}
const go = spawnSync("go", ["version"], { env, encoding: "utf8" });
if (go.status !== 0 || !go.stdout.includes("go1.27.1 ")) throw new Error("Go 1.27.1 and a native C compiler are required.");
prepareNative(root, target, env);
if (command === "build") {
  for (const file of ["ffmpeg-8.0.1-source.tar.gz", `ffmpeg-build-${target}.json`]) rmSync(join(root,"dist",target,file),{force:true});
}
if (["generate", "dev", "build"].includes(command)) {
  const defaults = spawnSync("go", ["run", "./cmd/contracts"], { cwd: root, env, stdio: "inherit" });
  if (defaults.error) throw new Error("Could not start the Go contract generator", { cause: defaults.error });
  if (defaults.status !== 0) process.exit(defaults.status ?? 1);
}
if (command === "test" && platform === "darwin") {
  env.CGO_LDFLAGS = `${env.CGO_LDFLAGS} -Wl,-rpath,${env.DAYBEE_NATIVE_LIBRARY_DIR}`;
}
const binary = ["test", "vet"].includes(command) ? "go" : binaryPath();
const flags = command === "test" ? ["test", ...args, "./..."] : command === "vet" ? ["vet", "./..."] : [command, ...args];
const child = spawnSync(binary, flags, { cwd: root, env, stdio: "inherit" });
if (child.error) throw new Error(`Could not start ${binary}`, { cause: child.error });
if (child.status === 0 && command === "build" && platform === "darwin") {
  const out = join(root, "dist", target);
  for (const file of readdirSync(out).filter(file => file.endsWith(".dmg") && !file.endsWith(`-${target}.dmg`))) {
    renameSync(join(out, file), join(out, file.replace(/\.dmg$/, `-${target}.dmg`)));
  }
}
process.exit(child.status ?? 1);
