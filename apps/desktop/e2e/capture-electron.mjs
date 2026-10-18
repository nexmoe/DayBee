// Run only against the pristine pre-migration checkout. Electron is deliberately
// not a project dependency; supply its archived executable explicitly.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { _electron } from "playwright";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { base, finishFrame, preload, visualCases } from "./visual-fixtures.mjs";

const root = process.env.DAYBEE_BASELINE_ROOT;
const executablePath = process.env.DAYBEE_ELECTRON_EXECUTABLE;
assert(root && executablePath, "DAYBEE_BASELINE_ROOT and DAYBEE_ELECTRON_EXECUTABLE are required");
assert(resolve(root) !== resolve(import.meta.dirname, ".."), "Use the unchanged Electron checkout");
const output = process.env.DAYBEE_BASELINE_OUTPUT;
assert(output, "DAYBEE_BASELINE_OUTPUT must be an explicit output directory");
const withoutFFmpeg = process.argv.includes("--without-ffmpeg");
const cases = visualCases().filter(item => !withoutFFmpeg || item.tab === 3);
const store = join(output, ".store"); mkdirSync(store, { recursive: true });
const settings = structuredClone(base.settings);
settings.desktop.startRecordingOnLaunch = false; settings.desktop.launchAtLogin = false;
writeFileSync(join(store, "desktop-settings.json"), JSON.stringify({ settings, windowState: { width: 420, height: 620 } }));
const app = await _electron.launch({ executablePath, args: [root], cwd: root,
  // The archived binary still needs its original isolation variables and bridge.
  env: { ...process.env, CI: "1", EVE_E2E_TEST: "1", EVE_STORE_DIR: store } });
try {
  const page = await app.firstWindow(); await page.waitForSelector("[role=tab]");
  console.log("Electron baseline page ready");
  const cdp = await page.context().newCDPSession(page);
  await page.addInitScript({ content: preload + `
    (() => {
      const track={stop(){},getSettings(){return {deviceId:'default'}}};
      const stream={getTracks:()=>[track],getAudioTracks:()=>[track]};
      Object.defineProperty(navigator, 'mediaDevices', {value:{
        getUserMedia:async()=>stream,
        enumerateDevices:async()=>${JSON.stringify(base.devices.map(d => ({ kind: "audioinput", deviceId: d.id, label: d.label })))},
        addEventListener(){},removeEventListener(){}
      }});
      window.AudioContext=class {
        state='running'; destination={}; sampleRate=16000;
        createMediaStreamSource(){return {connect(){},disconnect(){}}}
        createScriptProcessor(){return {connect(){},disconnect(){}}}
        createAnalyser(){return {frequencyBinCount:128,getByteFrequencyData(a){a.set(window.__daybeeSpectrum)},getFloatTimeDomainData(a){a.fill(0)}}}
        async close(){this.state='closed'}
      };
    })();` });
  await app.evaluate(({ BrowserWindow }) => {
    const contents = BrowserWindow.getAllWindows()[0].webContents;
    const send = contents.send.bind(contents);
    globalThis.__daybeeFixtureSend = snapshot => send("desktop:snapshot", snapshot);
    contents.send = (channel, ...args) => { if (channel !== "desktop:snapshot") send(channel, ...args); };
  });
  const inject = async snapshot => {
    await app.evaluate((_, fixture) => globalThis.__daybeeFixtureSend(fixture), snapshot);
    await page.waitForTimeout(80);
  };
  for (const item of cases) {
    console.log("Capturing", item.name);
    await page.evaluate(value => window.eve.saveSettings(value), item.snapshot.settings);
    await page.emulateMedia({ colorScheme: item.snapshot.settings.desktop.theme });
    await page.reload();
    await page.waitForFunction(() => Boolean(document.querySelector("[role=tab]")), {polling:100});
    await inject(item.snapshot);
    await page.evaluate(index => {
      const e=document.querySelectorAll('[role=tab]')[index];
      e.dispatchEvent(new MouseEvent('mousedown',{bubbles:true,button:0}));
      e.dispatchEvent(new MouseEvent('mouseup',{bubbles:true,button:0})); e.click();
    }, item.tab);
    if (item.fromRecording) {
      const recording = structuredClone(item.snapshot); recording.status.recording = true;
      await inject(recording); await page.evaluate(() => window.__daybeeStep(60)); await inject(item.snapshot);
    }
    if (item.dropdown) await page.evaluate(() => document.querySelector('[role=combobox]').dispatchEvent(new PointerEvent('pointerdown', {bubbles:true,button:0,pointerType:'mouse',ctrlKey:false})));
    await page.waitForTimeout(80);
    if (withoutFFmpeg) await page.evaluate(() => {
      const titles = [...document.querySelectorAll('p')].filter(e => e.textContent === 'ffmpeg');
      if (titles.length !== 1) throw Error('Expected exactly one original FFmpeg status row');
      titles[0].parentElement.parentElement.remove();
    });
    const evidence = await page.evaluate(finishFrame);
    assert.equal(evidence.dark, item.snapshot.settings.desktop.theme === "dark");
    assert.equal(evidence.hidden, false);
    const path = join(output, item.name); mkdirSync(resolve(path, ".."), { recursive: true });
    writeFileSync(path + ".json", JSON.stringify(evidence) + "\n");
    const screenshot = await cdp.send("Page.captureScreenshot", {format:"png",captureBeyondViewport:false});
    writeFileSync(path + ".png", Buffer.from(screenshot.data, "base64"));
  }
  if (withoutFFmpeg) {
    const target = process.platform === 'win32' ? 'windows-amd64' : `darwin-${process.arch === 'x64' ? 'amd64' : 'arm64'}`;
    const original = JSON.parse(readFileSync(new URL(`./baseline/electron-${target}/manifest.json`,import.meta.url),'utf8'));
    const sha256 = {};
    for (const item of cases) for (const ext of ['json','png']) {
      const path = item.name+'.'+ext;
      sha256[path] = createHash('sha256').update(readFileSync(join(output,path))).digest('hex');
    }
    writeFileSync(join(output,'manifest.json'),JSON.stringify({sourceCommit:original.sourceCommit,approvedChange:'remove-ffmpeg-status',sha256},null,2)+'\n');
  }
  console.log(`Captured ${cases.length} original Electron fixtures${withoutFFmpeg?' with the user-approved FFmpeg row removed':''} in ${output}`);
  if (process.argv.includes("--hold")) {
    const position = process.env.DAYBEE_BASELINE_COMPARE_POSITION
      ? JSON.parse(process.env.DAYBEE_BASELINE_COMPARE_POSITION) : null;
    assert(position === null || (Array.isArray(position) && position.length === 2 && position.every(Number.isInteger)));
    const windowState = await app.evaluate(({ BrowserWindow }, point) => {
      const window = BrowserWindow.getAllWindows()[0];
      if (point) window.setPosition(point[0], point[1], false);
      window.show(); window.focus();
      return { bounds: window.getBounds(), min: window.getMinimumSize(), max: window.getMaximumSize() };
    }, position);
    writeFileSync(join(output, "native-window-state.json"), JSON.stringify(windowState, null, 2) + "\n");
    console.log("Keeping the isolated original window open for native comparison.");
    await Promise.race([
      delay(10 * 60_000, undefined, { ref: false }),
      new Promise(resolve => process.once("SIGTERM", resolve))
    ]);
  }
} finally { await app.close(); }
