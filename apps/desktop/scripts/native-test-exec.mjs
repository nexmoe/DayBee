import { spawnSync } from "node:child_process";
import { copyFileSync, readdirSync } from "node:fs";
import { dirname, join } from "node:path";

const [binary, ...args] = process.argv.slice(2);
const library = process.env.DAYBEE_NATIVE_LIBRARY_DIR;
if (!binary || !library) throw new Error("Native tests require their executable and locked library directory");
// Windows searches the executable's directory before System32 and PATH.
// Go puts test executables in temporary directories, unlike packaged apps.
for (const file of readdirSync(library).filter(file => file.endsWith(".dll"))) {
  copyFileSync(join(library, file), join(dirname(binary), file));
}
const child = spawnSync(binary, args, { stdio: "inherit" });
if (child.error) throw child.error;
process.exit(child.status ?? 1);
