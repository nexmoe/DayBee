import type { DesktopSnapshot, RecordingHistoryItem } from "@daybee/shared";
import { expect, test } from "./fixtures";

const rounds = 40;
const reactMeasureLimit = 1000;
const customMark = "daybee-dev-performance-mark";
const customMeasure = "daybee-dev-performance-measure";
const at = (seconds: number) => new Date(Date.UTC(2026, 9, 12, 0, 0, seconds)).toISOString();

const record = (text: string): RecordingHistoryItem => ({
  id: "synthetic-dev-performance",
  audioSource: "microphone",
  sessionId: "synthetic-dev-performance",
  audioPath: "/recordings/synthetic-dev-performance.flac",
  folderPath: "/recordings",
  inputDevice: null,
  status: "ok",
  startedAt: at(0),
  textPreview: text,
  transcriptChunks: [{ text, startedAt: at(0), endedAt: at(4) }]
});

test("main window keeps React dev measures bounded while updates render", async ({ launchApp }) => {
  test.setTimeout(90_000);
  const { app, page } = await launchApp();
  await page.addInitScript(({ customMark, customMeasure }) => {
    const tracks = new Set(["Components ⚛", "Scheduler ⚛"]);
    const devtoolsOf = (entry: PerformanceEntry) => {
      const detail = (entry as PerformanceMeasure).detail as { devtools?: { track?: string; trackGroup?: string } } | undefined;
      return detail && typeof detail === "object" ? detail.devtools : undefined;
    };
    const state = { observed: 0, components: 0, scheduler: 0 };
    (window as unknown as { __daybeeDevPerformance: typeof state }).__daybeeDevPerformance = state;
    new PerformanceObserver(list => {
      for (const entry of list.getEntries()) {
        const devtools = devtoolsOf(entry);
        const components = entry.name === "Components ⚛" || devtools?.track === "Components ⚛";
        const scheduler = entry.name === "Scheduler ⚛" || devtools?.track === "Scheduler ⚛" || devtools?.trackGroup === "Scheduler ⚛";
        if (!components && !scheduler && !tracks.has(entry.name)) continue;
        state.observed += 1;
        if (components) state.components += 1;
        if (scheduler) state.scheduler += 1;
      }
    }).observe({ type: "measure", buffered: false });
    performance.mark(customMark);
    performance.measure(customMeasure, customMark);
  }, { customMark, customMeasure });

  const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
  snapshot.settings.desktop = { ...snapshot.settings.desktop, language: "zh-CN", voiceprintSetup: "skipped" };
  snapshot.status = { ...snapshot.status, recording: true, recovering: false, error: null };
  snapshot.history = [record("synthetic turn 0")];
  await page.goto("/?window=main");
  await page.setViewportSize({ width: 1000, height: 800 });
  const main = page.locator("[data-window=main]");
  const timeline = page.getByRole("list", { name: "转写时间线" });
  await expect(main).toBeVisible();

  let previousText = "";
  for (let round = 1; round <= rounds; round += 1) {
    const text = `synthetic turn ${round}`;
    const elapsed = `09:41:${String(round).padStart(2, "0")}`;
    snapshot.history = [record(text)];
    snapshot.status.elapsed = elapsed;
    snapshot.status.recording = true;
    await app.setFixture(snapshot);
    await expect(timeline.getByText(text, { exact: true })).toBeVisible();
    await expect(page.getByText(`录音中 · ${elapsed}`, { exact: true })).toBeVisible();
    if (previousText) await expect(timeline.getByText(previousText, { exact: true })).toHaveCount(0);
    previousText = text;
  }

  const counts = await page.evaluate(({ customMark, customMeasure }) => {
    const devtoolsOf = (entry: PerformanceEntry) => {
      const detail = (entry as PerformanceMeasure).detail as { devtools?: { track?: string; trackGroup?: string } } | undefined;
      return detail && typeof detail === "object" ? detail.devtools : undefined;
    };
    let components = 0;
    let scheduler = 0;
    for (const entry of performance.getEntriesByType("measure")) {
      const devtools = devtoolsOf(entry);
      if (entry.name === "Components ⚛" || devtools?.track === "Components ⚛") components += 1;
      if (entry.name === "Scheduler ⚛" || devtools?.track === "Scheduler ⚛" || devtools?.trackGroup === "Scheduler ⚛") scheduler += 1;
    }
    const observed = (window as unknown as { __daybeeDevPerformance: { observed: number; components: number; scheduler: number } }).__daybeeDevPerformance;
    return {
      components,
      scheduler,
      buffered: components + scheduler,
      observed: observed.observed,
      observedComponents: observed.components,
      observedScheduler: observed.scheduler,
      customMarks: performance.getEntriesByName(customMark, "mark").length,
      customMeasures: performance.getEntriesByName(customMeasure, "measure").length,
      text: document.body.innerText.length
    };
  }, { customMark, customMeasure });
  console.log(JSON.stringify(counts));

  expect(counts.observed, `observer received ${counts.observed} React measures`).toBeGreaterThan(0);
  expect(counts.observedComponents, "Components ⚛ profiling stayed enabled").toBeGreaterThan(0);
  expect(counts.buffered, `Components ${counts.components} Scheduler ${counts.scheduler}`).toBeLessThanOrEqual(reactMeasureLimit);
  expect(counts.components).toBeLessThanOrEqual(reactMeasureLimit);
  expect(counts.scheduler).toBeLessThanOrEqual(reactMeasureLimit);
  expect(counts.customMarks).toBe(1);
  expect(counts.customMeasures).toBe(1);
  expect(counts.text).toBeGreaterThan(0);
  await expect(main).toBeVisible();
  await expect(timeline.getByText("synthetic turn 40", { exact: true })).toBeVisible();
  await expect(page.getByText("录音中 · 09:41:40", { exact: true })).toBeVisible();
});
