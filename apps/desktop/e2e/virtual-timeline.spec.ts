import type { DesktopSnapshot, RecordingHistoryItem } from "@daybee/shared";
import type { Locator, Page } from "@playwright/test";
import { expect, test } from "./fixtures";

const count = 1000;
const baseTime = Date.parse("2026-10-09T10:00:00Z");
const label = (index: number) => `Timeline entry ${String(index).padStart(4, "0")}.`;
const record = (index: number): RecordingHistoryItem => ({
  id: `virtual-${index}`, sessionId: `session-${index}`, audioSource: "microphone",
  audioPath: `/recordings/${index}.flac`, folderPath: "/recordings", inputDevice: null, status: "ok",
  startedAt: new Date(baseTime - index * 120_000).toISOString(), transcriptChunks: [],
  textPreview: `${label(index)} ${"Variable height transcript with 中文和 English. ".repeat(Math.abs(index) % 4 + 1)}`
});

async function visibleAnchor(timeline: Locator) {
  let anchor: { text: string; top: number } | null = null;
  // A resize can briefly replace the rendered range. Wait for the next range
  // before comparing its reading position instead of aborting the poll.
  await expect.poll(async () => {
    anchor = await timeline.locator(".transcript-event").evaluateAll(elements => {
      const viewport = elements[0]?.closest("[data-radix-scroll-area-viewport]");
      if (!viewport) return null;
      const bounds = viewport.getBoundingClientRect();
      const element = elements.find(element => {
        const rect = element.getBoundingClientRect();
        return rect.top >= bounds.top && rect.top < bounds.bottom;
      });
      return element ? { text: element.querySelector(".transcript-text")!.textContent!, top: element.getBoundingClientRect().top } : null;
    });
    return anchor !== null;
  }).toBe(true);
  return anchor!;
}

async function expectMeasuredRows(timeline: Locator) {
  await expect.poll(() => timeline.locator(".transcript-event").evaluateAll(elements =>
    elements.flatMap((element, index) => {
      const next = elements[index + 1];
      const gap = next ? next.getBoundingClientRect().top - element.getBoundingClientRect().bottom : 0;
      return Math.abs(gap) <= 1 ? [] : [{ index: element.getAttribute("data-index"), gap }];
    })
  )).toEqual([]);
  expect(await timeline.locator(".transcript-event").count()).toBeLessThan(40);
}

async function openTimeline(page: Page, role: "compact" | "main") {
  if (role === "compact") {
    await page.goto("/");
    await page.setViewportSize({ width: 1000, height: 760 });
  }
  return page.getByRole("list", { name: /Transcription timeline|转写时间线/ });
}

for (const role of ["compact"] as const) {
  test(`virtualizes all history and preserves the reading position in the ${role} window`, async ({ launchApp }, testInfo) => {
    const { app, page } = await launchApp();
    const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    snapshot.history = Array.from({ length: count }, (_, index) => record(index));
    snapshot.microphoneActivity = { supported: true, error: null, items: [] };
    await app.setFixture(snapshot);
    const timeline = await openTimeline(page, role);
    const viewport = page.locator("[data-radix-scroll-area-viewport]").last();
    await expect(timeline.locator(".transcript-text").first()).toContainText(label(0));
    await expect(timeline.locator("li").first()).toHaveAttribute("aria-setsize", String(count));
    await expectMeasuredRows(timeline);
    // Dragging the scrollbar can jump over rows that have never been mounted.
    for (const fraction of [0.25, 0.75, 1]) {
      await viewport.evaluate((element, fraction) => { element.scrollTop = element.scrollHeight * fraction; }, fraction);
      await expect.poll(async () => Number(await timeline.locator("li").first().getAttribute("data-index")))
        .toBeGreaterThan(fraction * count - 80);
      await expectMeasuredRows(timeline);
    }
    await expect(timeline.locator(".transcript-event").last()).toContainText(label(count - 1));
    await expect(timeline.locator("[data-last=true]")).toBeInViewport();
    await viewport.evaluate(element => { element.scrollTop = element.scrollHeight / 2; });
    await expect.poll(async () => Number(await timeline.locator("li").first().getAttribute("data-index"))).toBeGreaterThan(350);
    await page.setViewportSize({ width: role === "compact" ? 380 : 720, height: 480 });
    await expect.poll(() => viewport.evaluate(element => element.clientWidth)).toBe(role === "compact" ? 380 : 720);
    await viewport.evaluate(async () => {
      await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    });
    await expectMeasuredRows(timeline);
    await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    const anchor = await visibleAnchor(timeline);
    snapshot.history.unshift(record(-1));
    await app.setFixture(snapshot);
    await expect(timeline.locator("li").first()).toHaveAttribute("aria-setsize", String(count + 1));
    await expect.poll(async () => Math.abs((await visibleAnchor(timeline)).top - anchor.top)).toBeLessThan(2);
    expect((await visibleAnchor(timeline)).text).toBe(anchor.text);
    snapshot.microphoneActivity.supported = false;
    await app.setFixture(snapshot);
    await expect(page.locator(".microphone-notice")).toHaveCount(1);
    await expect.poll(async () => Math.abs((await visibleAnchor(timeline)).top - anchor.top)).toBeLessThan(2);
    expect((await visibleAnchor(timeline)).text).toBe(anchor.text);
    snapshot.microphoneActivity.supported = true;
    await app.setFixture(snapshot);
    await expect(page.locator(".microphone-notice")).toHaveCount(0);
    await expect.poll(async () => Math.abs((await visibleAnchor(timeline)).top - anchor.top)).toBeLessThan(2);
    await viewport.evaluate(element => element.scrollTo(0, 0));
    await expect(timeline.locator(".transcript-text").first()).toContainText(label(-1));
    snapshot.history.unshift(record(-2));
    await app.setFixture(snapshot);
    await expect(timeline.locator(".transcript-text").first()).toContainText(label(-2));
    expect(await viewport.evaluate(element => element.scrollTop)).toBe(0);
    await expectMeasuredRows(timeline);
    await page.screenshot({ path: testInfo.outputPath(`virtual-timeline-${role}.png`), animations: "disabled" });
    if (role === "compact") {
      await viewport.evaluate(element => { element.scrollTop = element.scrollHeight / 2; });
      await page.setViewportSize({ width: 1000, height: 760 });
      await page.getByRole("tablist").getByRole("tab", { name: /General|通用/, exact: true }).click();
      await expect(timeline.locator("li")).toHaveCount(0);
      await page.getByRole("tablist").getByRole("tab", { name: /Home|首页/, exact: true }).click();
      await expect(timeline.locator(".transcript-text").first()).toContainText(label(-2));
      await expect.poll(() => viewport.evaluate(element => element.scrollTop)).toBe(0);
    }
    await app.close();
  });
}
