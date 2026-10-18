import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { verifyUpdate } from "./verify-update.mjs";

const target = process.env.DAYBEE_BUILD_TARGET || (process.platform === "win32" ? "windows-amd64" : `darwin-${process.arch === "x64" ? "amd64" : "arm64"}`);
const root = resolve("dist", target);
const mac = target.startsWith("darwin");
const productName = JSON.parse(readFileSync("package.json", "utf8")).productName;
const bundle = join(root, `${productName}.app`);
const resources = mac ? join(bundle, "Contents/Resources") : root;
const binary = mac ? join(bundle, "Contents/MacOS", productName) : join(root, `${productName}.exe`);
assert(existsSync(binary), "Missing executable");
for (const lib of ["sherpa-onnx-c-api", "sherpa-onnx-cxx-api", "onnxruntime"]) {
  assert(existsSync(join(resources, mac ? `lib/lib${lib}.dylib` : `${lib}.dll`)), `Missing ${lib}`);
}
for (const license of ["sherpa-onnx", "onnxruntime", "flac", "icza-bitio", "mewkiz-pkg", "mewpkg-term", "inter", "aishell-3", "aishell-3-NOTICE"]) {
  assert(readFileSync(join(resources, "licenses", license + ".txt")).length > 100, `Missing ${license} license`);
}
const versions = JSON.parse(readFileSync(join(resources, "native-versions.json"), "utf8"));
assert.equal(versions.sherpa, "1.13.8"); assert.equal(versions.onnxruntime, "1.28.2");
assert.equal(versions.flac, "1.0.14");
const inspect = path => {
  for (const item of readdirSync(path, { withFileTypes: true })) {
    assert(!/^(node_modules|Electron Framework\.framework|app\.asar|node\.exe|bun\.exe|ffmpeg(?:[.-].*)?|ffprobe(?:[.-].*)?)$/.test(item.name), `Unexpected old runtime: ${item.name}`);
    if (item.isDirectory()) inspect(join(path, item.name));
  }
}; inspect(root);
if (mac) {
  const run = (cmd, args) => execFileSync(cmd, args, { encoding: "utf8" });
  const paths = run("otool", ["-l", binary]);
  assert(paths.includes("@executable_path/../Resources/lib"), "Missing bundle rpath");
  assert(!paths.includes("/pkg/mod/") && !paths.includes("/.generated/"), "Build-machine rpath leaked into executable");
  for (const path of [binary, ...readdirSync(join(resources, "lib")).map(name => join(resources, "lib", name))]) {
    // --deep on the app does not verify code nested in Resources/lib.
    run("codesign", ["--verify", "--strict", path]);
    const info = run("otool", ["-l", path]);
    const min = info.match(/\bminos (\d+)\.(\d+)/);
    assert(min, `Missing minimum OS in ${path}`);
    assert(Number(min[1]) < 13 || (Number(min[1]) === 13 && Number(min[2]) <= 3), `${path} requires macOS newer than 13.3`);
  }
  run("codesign", ["--verify", "--deep", "--strict", bundle]);
}
const env = { ...process.env }; delete env.DYLD_LIBRARY_PATH;
const diagnosticStore = mkdtempSync(join(tmpdir(), "daybee-package-native-"));
try {
  const loaded = JSON.parse(execFileSync(binary, [], {
    env: { ...env, DAYBEE_VERIFY_NATIVE: "1", DAYBEE_STORE_DIR: diagnosticStore, DAYBEE_E2E_TEST: "1" },
    encoding: "utf8", windowsHide: true, timeout: 15000, killSignal: "SIGKILL"
  }));
  assert.deepEqual(loaded, { sherpa: "1.13.8", onnxruntime: "1.28.2" }, "Loaded native versions differ from package pins");
} finally { rmSync(diagnosticStore, { recursive: true, force: true }); }
const scratch=mkdtempSync(join(tmpdir(),"daybee-package-audio-"));
try {
  const source=join(scratch,"source.wav"), flac=join(scratch,"audio.flac");
  const wav=Buffer.alloc(44+48000*4);
  wav.write("RIFF");wav.writeUInt32LE(wav.length-8,4);wav.write("WAVEfmt ",8);wav.writeUInt32LE(16,16);
  wav.writeUInt16LE(1,20);wav.writeUInt16LE(2,22);wav.writeUInt32LE(48000,24);wav.writeUInt32LE(192000,28);
  wav.writeUInt16LE(4,32);wav.writeUInt16LE(16,34);wav.write("data",36);wav.writeUInt32LE(wav.length-44,40);
  for(let i=0;i<48000;i++) for(let channel=0;channel<2;channel++) wav.writeInt16LE(Math.round(Math.sin(2*Math.PI*440*i/48000)*(channel?0.2:0.4)*32767),44+i*4+channel*2);
  writeFileSync(source,wav);
  const diagnostic = args => JSON.parse(execFileSync(binary, args, {
    env: {...env, PATH:"", DAYBEE_VERIFY_AUDIO:"1"}, encoding:"utf8", windowsHide:true, timeout:15000
  }));
  const decoded=diagnostic(["encode",source,flac]);
  assert.equal(decoded.sampleRate,16000); assert.equal(decoded.samples.length,16000);
  const md5=createHash("md5").update(wav.subarray(44)).digest();
  assert.deepEqual(readFileSync(flac).subarray(26,42),md5,"FLAC STREAMINFO checksum differs from original PCM");
  let power=0,crossings=0,last=0;
  for(const value of decoded.samples){power+=value*value;if(last<0&&value>=0)crossings++;last=value;}
  assert(Math.abs(Math.sqrt(power/16000)-0.3/Math.sqrt(2))<0.005,"Resampling changed signal level");
  assert(Math.abs(crossings-440)<=1,"Resampling changed signal frequency");
  const fixtures=JSON.parse(readFileSync("internal/codec/testdata/manifest.json","utf8"));
  for(const [name,fixture] of Object.entries(fixtures)) {
    const path=resolve("internal/codec/testdata",fixture.file);
    assert.equal(createHash("sha256").update(readFileSync(path)).digest("hex"),fixture.sha256,"Independent audio fixture changed");
    const result=diagnostic(["decode",path]);
    assert.equal(result.sampleRate,16000);
    assert.equal(result.samples.length,Math.ceil(fixture.samples*16000/fixture.rate));
    if(name==="legacy-mono") result.samples.forEach((value,i)=>assert(Math.abs(value-((i*7919+12345)%65536-32768)/32768)<1e-7,"Legacy FLAC PCM changed"));
  }
} finally {rmSync(scratch,{recursive:true,force:true});}
if (existsSync(join(root,`update-${target}.json`))) verifyUpdate(root,target);
console.log(`${target}: package, licenses, native loading, pure Go FLAC, legacy recordings and 16 kHz resampling passed.`);
