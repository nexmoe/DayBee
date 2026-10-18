import { readFileSync } from "node:fs";

export const base = JSON.parse(readFileSync(new URL("./baseline/snapshot.json", import.meta.url), "utf8"));
export const preload = readFileSync(new URL("../internal/desktop/visual-preload.js", import.meta.url), "utf8");
export const spectrum = Array.from({ length: 128 }, (_, i) => (i * 29 + 17) % 256);
export function visualCases() {
  const cases = [];
  for (const language of ["en-US", "zh-CN"]) for (const theme of ["light", "dark"]) {
    const snapshot = structuredClone(base);
    snapshot.settings.desktop.language = language; snapshot.settings.desktop.theme = theme;
    const add = (name, tab = 0, patch = {}, extra = {}) => {
      const fixture = structuredClone(snapshot); Object.assign(fixture.status, patch);
      cases.push({ name: `${language}-${theme}/${name}`, tab, snapshot: fixture, ...extra });
    };
    ["history", "general", "input", "transcribe"].forEach((name, index) => add(name, index));
    const recording = { recording: true, inSpeech: true, asrPreview: "固定中文转写 / Fixed transcription.", elapsed: "00:01:23" };
    add("recording", 0, recording);
    add("transcribing", 0, { ...recording, inSpeech: false });
    add("pure-recording", 0, { ...recording, asrEnabled: false, asrPreview: "" });
    add("stopped", 0, {}, { fromRecording: true });
    add("download", 0, { downloading: true, downloadProgress: 42, downloadMessage: "Downloading Qwen3 ASR model", senseVoiceReady: false });
    add("error", 0, { error: "Fixed test error.", statusMessage: "Fixed test error." });
    add("permission", 0, {}, { permission: { state: "denied", supported: true, message: "麦克风权限已拒绝，请到系统设置手动开启。" } });
    add("populated-history", 0, {}, { history: [{ id: "/tmp/fixed.wav", audioPath: "/tmp/fixed.wav", folderPath: "/tmp", inputDevice: "MacBook Pro Microphone", startedAt: "2026-10-09T12:00:00+08:00", status: "ok", textPreview: "固定中文转写 / Fixed transcription." }] });
    add("dropdown", 1, {}, { dropdown: true });
  }
  return cases.map(item => {
    if (item.permission) item.snapshot.permission = item.permission;
    if (item.history) item.snapshot.history = item.history;
    return item;
  });
}

export const finishFrame = `(() => {
  window.__daybeeStep(60);
  document.getAnimations().forEach(a => { if(a.effect.getComputedTiming().iterations===Infinity) a.cancel(); else a.finish(); });
  return {
    hidden: document.hidden, dark: matchMedia('(prefers-color-scheme: dark)').matches,
    dimensions: [innerWidth, innerHeight], canvas: window.__daybeeCanvasCommands,
    elements: [...document.querySelectorAll('button,input,[role],h2,h3,p,label')].map(e => {
      const r=e.getBoundingClientRect(),s=getComputedStyle(e);
      return {tag:e.tagName,role:e.getAttribute('role'),text:e.textContent,rect:[r.x,r.y,r.width,r.height],font:s.font,color:s.color};
    })
  };
})()`;
