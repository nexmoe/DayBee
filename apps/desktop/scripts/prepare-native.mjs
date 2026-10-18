import { execFileSync } from "node:child_process";
import { chmodSync, cpSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { basename, delimiter, join } from "node:path";

const version = "1.13.8";
const targets = {
  "darwin-arm64": ["macos", "aarch64-apple-darwin"],
  "darwin-amd64": ["macos", "x86_64-apple-darwin"],
  "windows-amd64": ["windows", "x86_64-pc-windows-gnu"]
};

export function prependNativePath(env, library, separator = delimiter) {
  const keys = Object.keys(env).filter(key => key.toLowerCase() === "path").sort();
  const key = keys[0] || "PATH";
  const previous = env[key];
  for (const duplicate of keys.slice(1)) delete env[duplicate];
  env[key] = [library, previous].filter(Boolean).join(separator);
}

export function prepareMacLibrary(source, bundled, { run = execFileSync, warn = console.warn } = {}) {
  const restore = () => {
    rmSync(bundled, { force: true });
    cpSync(source, bundled);
    chmodSync(bundled, 0o755);
    // Remove the upstream signature so mygo signs the bundled copy.
    run("codesign", ["--remove-signature", bundled]);
  };
  restore();
  try {
    run("xcrun", ["strip", "-x", bundled], { stdio: "inherit" });
  } catch (error) {
    // Some Xcode versions cannot rewrite the upstream ONNX __LINKEDIT layout.
    // Restore from the untouched cache: strip may have partially changed its copy.
    restore();
    warn(`Keeping unstripped ${basename(source)}: ${error.message}`);
  }
}

export function prepareNative(root, target, env) {
  const details = targets[target];
  if (!details) throw new Error(`Unsupported desktop target: ${target}`);
  const [platform, triple] = details;
  const run = (args) => execFileSync("go", args, { cwd: root, env, encoding: "utf8" }).trim();
  const module = `github.com/k2-fsa/sherpa-onnx-go-${platform}`;
  run(["mod", "download", `${module}@v${version}`]);
  const cache = run(["env", "GOMODCACHE"]);
  const source = join(cache, `${module}@v${version}`);
  const generated = join(root, ".generated");
  mkdirSync(generated, { recursive: true });
  const resource = join(root, "resources", target);
  mkdirSync(resource, { recursive: true });
  const licenses = join(resource, "licenses"); mkdirSync(licenses, { recursive: true });
  // Remove obsolete generated resources when rebuilding an existing checkout.
  for (const path of ["bin", "ffmpeg-build.json", "licenses/ffmpeg.txt"]) rmSync(join(resource,path),{recursive:true,force:true});
  cpSync(join(source, "LICENSE"), join(licenses, "sherpa-onnx.txt"));
  cpSync(join(root, "internal", "voiceprint", "LICENSE.txt"), join(licenses, "aishell-3.txt"));
  cpSync(join(root, "internal", "voiceprint", "NOTICE.txt"), join(licenses, "aishell-3-NOTICE.txt"));
  writeFileSync(join(resource, "native-versions.json"), JSON.stringify({sherpa:version,onnxruntime:"1.28.2",malgo:"0.11.26",flac:"1.0.14",target},null,2)+"\n");
  let library = join(source, "lib", triple);
  env.DAYBEE_NATIVE_LIBRARY_DIR = library;
  if (!existsSync(library)) throw new Error(`Missing locked sherpa libraries: ${library}`);
  if (platform === "macos") {
    // The upstream Go package embeds its module-cache rpath. Patch an isolated
    // copy before linking, so signed bundles/updates never depend on that cache.
    const copy = join(generated, `sherpa-${platform}-${version}`);
    if (!existsSync(join(copy, "go.mod"))) cpSync(source, copy, { recursive: true });
    for (const file of readdirSync(copy).filter((f) => /^build_darwin_.*\.go$/.test(f))) {
      const path = join(copy, file);
      chmodSync(path, 0o644);
      writeFileSync(path, readFileSync(path, "utf8").replaceAll(/-Wl,-rpath,\$\{SRCDIR\}\/lib\/\S+/g, "-Wl,-rpath,@executable_path/../Resources/lib"));
    }
    const mod = join(generated, "desktop.mod");
    writeFileSync(mod, readFileSync(join(root, "go.mod"), "utf8") + `\nreplace ${module} => ${JSON.stringify(copy)}\n`);
    cpSync(join(root, "go.sum"), join(generated, "desktop.sum"));
    env.GOFLAGS = `${env.GOFLAGS || ""} -mod=mod -modfile=${mod}`.trim();
    env.CGO_LDFLAGS_ALLOW = "-Wl,-rpath,@executable_path/\\.\\./Resources/lib";
    const dest = join(resource, "lib"); mkdirSync(dest, { recursive: true });
    for (const file of readdirSync(library).filter((f) => f.endsWith(".dylib"))) {
      prepareMacLibrary(join(library, file), join(dest, file));
    }
    env.DYLD_LIBRARY_PATH = [library, env.DYLD_LIBRARY_PATH].filter(Boolean).join(delimiter);
  } else {
    for (const file of readdirSync(library).filter((f) => f.endsWith(".dll"))) {
      const dest = join(resource, file);
      if (existsSync(dest)) chmodSync(dest, 0o666);
      rmSync(dest, { force:true }); cpSync(join(library,file),dest); chmodSync(dest,0o644);
    }
    prependNativePath(env, library);
  }
  return env;
}
