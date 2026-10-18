import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { randomBytes } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { setTimeout as delay } from "node:timers/promises";

const root = join(import.meta.dirname, "..");
const productName = JSON.parse(readFileSync(join(root, "package.json"), "utf8")).productName;
const capture = process.argv.includes("--capture");
const interactive = process.argv.includes("--interactive");
assert(!capture || interactive, "Visible native captures require --interactive; default tests stay in the background.");
assert(!process.argv.includes("--hold"), "Native tests use a mock engine. Launch normally for real recording.");
console.log("Native UI test: mock engine; microphone capture and inference are not exercised.");
const target = process.platform === "win32" ? "windows-amd64" : `darwin-${process.arch === "x64" ? "amd64" : "arm64"}`;
const executable = process.env.DAYBEE_TEST_EXECUTABLE || join(root, "dist", target,
  ...(process.platform === "darwin" ? [`${productName}.app`, "Contents", "MacOS", productName] : [`${productName}.exe`]));
assert(existsSync(executable), `Build the native app first: ${executable}`);
const store = mkdtempSync(join(tmpdir(), "daybee-workspace-native-"));
const token = randomBytes(32).toString("hex");
const port = process.env.DAYBEE_TEST_PORT || "39887";
const original = JSON.parse(readFileSync(join(root, "e2e/baseline/snapshot.json"), "utf8"));
original.app.name = productName;
original.settings.desktop.startRecordingOnLaunch = false;
original.settings.desktop.launchAtLogin = false;
original.settings.desktop.voiceprintSetup = "skipped";
original.settings.recording.outputDir = join(store, "recordings");
writeFileSync(join(store, "desktop-settings.json"), JSON.stringify({ settings: original.settings, nativeDevices: true }));
const env = { ...process.env, DAYBEE_E2E_TEST: "1", DAYBEE_TEST_BACKGROUND: interactive ? "0" : "1", DAYBEE_STORE_DIR: store, DAYBEE_TEST_PORT: port, DAYBEE_TEST_TOKEN: token };
delete env.DYLD_LIBRARY_PATH;
const child = spawn(executable, [], { env, stdio: ["ignore", "pipe", "pipe"] });
let logs = "";
child.stdout.on("data", b => { logs += b; }); child.stderr.on("data", b => { logs += b; });
const request = async (path, body, role = "compact") => {
  const response = await fetch(`http://127.0.0.1:${port}${path}?window=${role}`, {
    method: "POST", headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(15000)
  });
  if (!response.ok) throw new Error(`${path}: ${await response.text()}`);
  return path === "/capture" ? Buffer.from(await response.arrayBuffer()) : response.json();
};
const evaluate = (code, role = "compact") => request("/eval", { code }, role);
const call = (method, args = [], role = "compact") => evaluate(`window.mygo.call(${JSON.stringify("DesktopService." + method)}, ...${JSON.stringify(args)})`, role);
const click = (selector, role = "compact") => evaluate(`(() => {const e=document.querySelector(${JSON.stringify(selector)}); if(!e)throw Error('Missing control');e.click();return true})()`, role);
const navigate = index => evaluate(`(() => {const e=document.querySelectorAll('nav [data-sidebar="menu-button"]')[${index}]; if(!e)throw Error('Missing destination');e.click();return true})()`, "main");
const ready = async (role = "compact", previous) => {
  const deadline = Date.now() + 30000;
  while (true) {
    try {
      if (await evaluate(`Boolean(window.mygo && document.querySelector('[data-window=${role}]')) && window.__previousDocument!==${JSON.stringify(previous || "unassigned")}`, role)) break;
    } catch { /* Navigation/startup has not committed yet. */ }
    assert(Date.now() < deadline && child.exitCode === null, `Native ${role} did not become ready.\n${logs}`);
    await delay(100);
  }
  await evaluate("document.fonts.ready.then(() => true)", role);
};
const reload = async (role = "compact") => {
  const marker = randomBytes(16).toString("hex");
  await evaluate(`window.__previousDocument=${JSON.stringify(marker)}`, role);
  await request("/reload", undefined, role);
  await ready(role, marker);
};
try {
  await ready();
  assert.equal((await call("Bootstrap")).engineReady, true);
  assert.equal(await evaluate("document.querySelectorAll('[role=tab]').length"), 0);
  assert.deepEqual(await evaluate("({width:innerWidth,height:innerHeight})"), { width: 420, height: 620 });
  if (process.platform === "darwin") {
    assert.equal((await request("/window")).native.visibleWindowButtons, 0);
  }
  await call("StartRecording");
  await request("/hide");
  assert.equal((await request("/snapshot")).status.recording, true);
  await request("/show"); await reload();
  assert.equal((await call("Bootstrap")).status.recording, true);
  if (process.platform === "darwin") {
    await request("/hide-app");
    assert.equal((await request("/window")).appHidden, true);
    await request("/show");
    assert.equal((await request("/window")).appHidden, !interactive);
    assert.equal((await request("/window")).native.visibleWindowButtons, 0);
  }
  // Exercise the compact shortcut, rather than opening only via a test endpoint.
  await click(".open-workspace"); await ready("main");
  const first = await request("/window", undefined, "main");
  assert.equal(first.count, 2); assert.equal(first.visible, interactive);
  if (!interactive) assert.equal(first.focused, false);
  if (process.platform === "darwin" && interactive) assert.equal(first.native.visibleWindowButtons, 3);
  assert.deepEqual(await evaluate("({width:innerWidth,height:innerHeight})", "main"), { width: 1000, height: 760 });
  assert.equal(await evaluate("document.querySelectorAll('nav [data-sidebar=menu-button]').length", "main"), 5);
  await request("/speech", { text: "Native completed speech appears while recording." });
  const streamingDeadline = Date.now() + 1000;
  for (const role of ["compact", "main"]) {
    while (!await evaluate("Array.from(document.querySelectorAll('.transcript-text')).some(e => e.textContent.includes('Native completed speech appears while recording.'))", role)) {
      assert(Date.now() < streamingDeadline, `Completed speech did not stream to ${role}`);
      await delay(20);
    }
  }
  assert.equal((await call("Bootstrap")).status.recording, true);
  assert.equal(await evaluate("document.body.innerText.includes('Live transcript') || document.body.innerText.includes('实时转写')"), false);
  await navigate(2);
  assert.equal(await evaluate("document.querySelector('nav [aria-current=page]').getAttribute('aria-label') === document.querySelector('h1').textContent.trim()", "main"), true);
  await call("CloseWindow", ["main"], "main");
  assert.equal((await request("/window", undefined, "main")).visible, false);
  assert.equal((await call("Bootstrap")).status.recording, true);
  await call("OpenMainWindow");
  assert.equal((await request("/window", undefined, "main")).id, first.id);
  await call("OpenMainWindow");
  assert.equal((await request("/window", undefined, "main")).count, 2);
  await reload("main");
  assert.equal((await call("Bootstrap", [], "main")).status.recording, true);
  await call("StopRecording", [], "main");
  assert.equal((await call("Bootstrap")).status.recording, false);
  const settings = structuredClone(original.settings); settings.recording.disableAsr = true;
  await call("SaveSettings", [settings], "main"); await call("StartRecording");
  const pure = await call("Bootstrap", [], "main");
  assert.equal(pure.status.asrEnabled, false); assert.equal(pure.status.inSpeech, true);
  await call("StopRecording");
  console.log(`Native ${target}: compact shortcut, singleton main window, sidebar navigation, immediate transcripts in both windows, shared settings/recording, close/reopen/reload lifetime passed.`);
  if (capture) {
    const output = join(root, "e2e/results", target, "workspace"); mkdirSync(output, { recursive: true });
    const evidence = [];
    for (const language of ["zh-CN", "en-US"]) for (const theme of ["light", "dark"]) {
      const snapshot = structuredClone(original);
      snapshot.settings.desktop.language = language; snapshot.settings.desktop.theme = theme;
      await request("/fixture", snapshot); await delay(150);
      for (const role of ["compact", "main"]) {
        await request("/show", undefined, role);
        for (const page of role === "main" ? [0, 1, 2, 3, 4] : [0]) {
          if (role === "main") await navigate(page);
          await evaluate("document.fonts.ready.then(() => true)", role); await delay(400);
          const state = await evaluate("({hidden:document.hidden,scrollWidth:document.documentElement.scrollWidth,width:innerWidth,dark:matchMedia('(prefers-color-scheme: dark)').matches,heading:document.querySelector('h1')?.textContent,ffmpeg:document.body.innerText.toLowerCase().includes('ffmpeg')})", role);
          assert.equal(state.hidden, false, `Native page is hidden or occluded; keep the test window visible for capture: ${JSON.stringify(await request('/window', undefined, role))}`);
          assert.equal(state.scrollWidth, state.width); assert.equal(state.dark, theme === "dark"); assert.equal(state.ffmpeg, false);
          const name = `${language}-${theme}-${role}-${page}`;
          writeFileSync(join(output, name + ".png"), await request("/capture", undefined, role));
          evidence.push({ name, ...state });
        }
      }
    }
    writeFileSync(join(output, "checks.json"), JSON.stringify(evidence, null, 2) + "\n");
    console.log(`Captured ${evidence.length} visible bilingual light/dark workspace screenshots in ${output}. Previous Electron layout baselines are preserved.`);
  }
} finally {
  child.kill("SIGTERM");
  await Promise.race([new Promise(resolve => child.once("exit", resolve)), delay(10000, undefined, { ref: false }).then(() => child.kill("SIGKILL"))]);
  if (logs) writeFileSync(join(store, "native.log"), logs);
}
