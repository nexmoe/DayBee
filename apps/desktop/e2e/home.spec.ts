import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import type { AppLanguage, DesktopSnapshot } from "@daybee/shared";
import { expect, test } from "./fixtures";

async function openHome(page: Page) {
  await page.goto("/?window=main");
  await page.setViewportSize({ width: 1000, height: 760 });
  await expect(page.getByRole("heading", { name: /Home|首页/, exact: true })).toBeVisible();
  return page.getByRole("region", { name: /Home|首页/, exact: true });
}

async function outputDirectory(page: Page) {
  const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
  const directory = snapshot.settings.recording.outputDir;
  mkdirSync(directory, { recursive: true });
  return directory;
}

function saveTranscript(directory: string, name: string, at: string, text: string) {
  writeFileSync(join(directory, `${name}.json`), JSON.stringify({
    audio_path: join(directory, `${name}.flac`), segment_start_time: at,
    status: text.trim() ? "ok" : "audio_only", text, input_device: "USB Podcast Mic"
  }));
}

test("opens on Home and explains the empty timeline", async ({ launchApp }) => {
  const { app, page } = await launchApp();
  const home = await openHome(page);
  await expect(page.getByRole("navigation").getByRole("button", { name: /Home|首页/, exact: true }))
    .toHaveAttribute("aria-current", "page");
  await expect(home.getByRole("heading", { name: /No activity yet|还没有活动记录/ })).toBeVisible();
  await expect(home.getByRole("button", { name: /Recording controls|录音控制/ })).toBeVisible();
  await expect(home.getByRole("article")).toHaveCount(0);
  await app.close();
});

test("merges segments, reflows complete paragraphs and orders them by actual time", async ({ launchApp }) => {
  const { app, page } = await launchApp();
  const directory = await outputDirectory(page);
  const older = "Oldest complete transcript. ".repeat(30).trim();
  const earlier = "Earlier actual time. ".repeat(30).trim();
  const fullText = `完整的会议转写正文。${"这里保留每一个转写细节，不截断内容。".repeat(100)}\n\n第二段保留换行。\n${"LongUnbrokenWord".repeat(24)}\n全文结束标记🙂。`;
  // Filesystem order and the timestamp strings both differ from chronological order.
  saveTranscript(directory, "a-wall-clock-later", "2026-10-09T10:00:00+09:00", earlier);
  saveTranscript(directory, "m-newest", "2026-10-09T09:15:00+06:00", fullText);
  saveTranscript(directory, "z-older", "2026-10-08T23:30:00+08:00", older);
  saveTranscript(directory, "pending-audio", "2026-10-10T12:00:00Z", "   ");
  await app.refreshHistory();
  const home = await openHome(page);
  const entries = home.getByRole("article");
  await expect.poll(() => entries.count()).toBeGreaterThan(3);
  const viewport = page.locator("[data-radix-scroll-area-viewport]").last();
  const total = Number(await home.locator(".transcript-event").first().getAttribute("aria-setsize"));
  const collected = new Map<number, { time: string; text: string }>();
  // Verify the complete transcript by reading each virtualized section in order.
  for (let pass = 0; pass < 100 && collected.size < total; pass++) {
    const visible = await home.locator(".transcript-event").evaluateAll(elements => elements.map(element => ({
      index: Number(element.getAttribute("data-index")),
      time: Array.from(element.querySelectorAll("time")).at(-1)?.dateTime ?? "",
      text: Array.from(element.querySelectorAll(".transcript-text")).map(text => text.textContent).join("")
    })));
    visible.forEach(({ index, ...row }) => collected.set(index, row));
    if (collected.size < total) await viewport.evaluate(async element => {
      element.scrollTop += element.clientHeight / 2;
      await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    });
  }
  expect(collected.size).toBe(total);
  let time = "";
  const rows = [...collected].sort(([a], [b]) => a - b).map(([, row]) => {
    if (row.time) time = row.time;
    return { ...row, time };
  });
  const times = rows.map(row => row.time);
  expect([...new Set(times)]).toEqual(["2026-10-09T03:15:00.000Z", "2026-10-09T01:00:00.000Z", "2026-10-08T15:30:00.000Z"]);
  const chronologicalText = rows.sort((a, b) => Date.parse(a.time) - Date.parse(b.time)).map(row => row.text).join("");
  expect(chronologicalText.replace(/\s/gu, "")).toBe(`${older}${earlier}${fullText}`.replace(/\s/gu, ""));
  await expect(home).not.toContainText("m-newest.flac");
  expect(await entries.locator(".transcript-paragraph").evaluateAll(elements => elements.every(element =>
    element.clientHeight >= element.scrollHeight && getComputedStyle(element).whiteSpace === "normal"
  ))).toBe(true);
  await entries.last().scrollIntoViewIfNeeded();
  await page.getByRole("navigation").getByRole("button", { name: /General|通用/, exact: true }).click();
  await expect(page.getByRole("heading", { name: /General|通用/, exact: true })).toBeInViewport();
  await page.getByRole("navigation").getByRole("button", { name: /Home|首页/, exact: true }).click();
  await expect(page.getByRole("heading", { name: /Home|首页/, exact: true })).toBeInViewport();
  await page.setViewportSize({ width: 720, height: 540 });
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await expect(home.locator(".transcript-event").first()).toHaveAttribute("aria-setsize", String(rows.length));
  await expect(entries.first()).toContainText("完整的会议转写正文。");
  await app.close();
});

test("adds completed transcripts without reloading and keeps daily history separate", async ({ launchApp }) => {
  const { app, page } = await launchApp();
  const directory = await outputDirectory(page);
  saveTranscript(directory, "first", "2026-10-09T10:00:00Z", "今天讨论录音");
  saveTranscript(directory, "second", "2026-10-09T10:00:30Z", "");
  await app.refreshHistory();
  const home = await openHome(page);
  await expect(home.getByRole("article")).toHaveCount(1);
  saveTranscript(directory, "second", "2026-10-09T10:00:30Z", "质量和转写结果。");
  await app.refreshHistory();
  await expect(home.getByRole("article").locator(".transcript-text"))
    .toHaveText(["质量和转写结果。", "今天讨论录音"]);
  expect(await home.locator("time").evaluateAll(elements => elements.map(element => element.getAttribute("datetime"))))
    .toEqual(["2026-10-09T10:00:30.000Z", "2026-10-09T10:00:00.000Z"]);
  await page.getByRole("navigation").getByRole("button", { name: /History|历史/, exact: true }).click();
  const history = page.getByRole("region", { name: /History|历史/, exact: true });
  await expect(history.getByText(/2 recordings · 2 transcribed|2 段录音 · 2 段已转写/)).toBeVisible();
  await page.getByRole("navigation").getByRole("button", { name: /Home|首页/, exact: true }).click();
  await expect(home.getByRole("article").locator(".transcript-text"))
    .toHaveText(["质量和转写结果。", "今天讨论录音"]);
  await app.close();
});

for (const language of ["zh-CN", "en-US"] as const) for (const theme of ["light", "dark"] as const) {
  test(`shows the timeline in ${language} ${theme} at desktop and narrow sizes`, async ({ launchApp }, testInfo) => {
    const { app, page } = await launchApp();
    await page.emulateMedia({ colorScheme: theme });
    const directory = await outputDirectory(page);
    saveTranscript(directory, "latest", "2026-10-09T11:00:00Z", "这是最近一次会议的完整转写。\n\nWe agreed to finish the recording review this afternoon.");
    saveTranscript(directory, "earlier", "2026-10-09T10:00:00Z", "上一段转写记录。\nThe previous conversation remains available in full.");
    await app.refreshHistory();
    await page.evaluate(async ({ language, theme }: { language: AppLanguage; theme: "light" | "dark" }) => {
      const snapshot = await window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap");
      await window.mygo!.call("DesktopService.SaveSettings", {
        ...snapshot.settings, desktop: { ...snapshot.settings.desktop, language, theme }
      });
    }, { language, theme });
    const home = await openHome(page);
    await expect(home.getByRole("article")).toHaveCount(3);
    await expect(page.locator("html")).toHaveClass(theme === "dark" ? /dark/ : /^$/);
    await expect(page.getByRole("heading", { name: language === "zh-CN" ? "首页" : "Home", exact: true })).toBeVisible();
    await expect(page.locator(".workspace-toolbar h1")).toHaveText(language === "zh-CN" ? "首页" : "Home");
    await expect(page.getByRole("searchbox", { name: language === "zh-CN" ? "搜索转写或应用" : "Search transcripts or apps" })).toBeVisible();
    await expect(page.getByRole("heading", { level: 1 })).toHaveCount(1);
    await expect(page.locator(".workspace-brand")).toHaveText("DayBee");
    expect(await page.locator(".workspace-logo").evaluate(element =>
      element instanceof HTMLImageElement && element.complete && element.naturalWidth > 0
    )).toBe(true);
    // The native window supplies this backdrop beneath the transparent renderer.
    await page.addStyleTag({ content: `html { background: ${theme === "dark" ? "#1c1c1e" : "#f5f5f7"}; }` });
    expect(await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--foreground").trim()))
      .toBe(theme === "dark" ? "rgba(255, 255, 255, 0.9)" : "rgba(0, 0, 0, 0.85)");
    for (const [size, width, height] of [["desktop", 1000, 760], ["narrow", 720, 540]] as const) {
      await page.setViewportSize({ width, height });
      await page.evaluate(() => document.fonts.ready);
      await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      await expect.poll(() => home.locator(".transcript-event").evaluateAll(elements => elements.every((element, index) => {
        const next = elements[index + 1];
        return !next || Math.abs(next.getBoundingClientRect().top - element.getBoundingClientRect().bottom) <= 1;
      }))).toBe(true);
      const heading = await page.getByRole("heading", { level: 1 }).boundingBox();
      const trigger = await page.getByRole("button", { name: /Toggle sidebar|切换侧栏/ }).boundingBox();
      expect(Math.abs(heading!.y + heading!.height / 2 - trigger!.y - trigger!.height / 2)).toBeLessThan(1);
      await expect(page.getByTestId("voiceprint-guide")).toBeVisible();
      await expect(page.getByTestId("speech-activity")).toBeVisible();
      await expect(home.locator("time").first()).toBeVisible();
      // Read one coherent layout frame; fonts and resize transitions can move
      // sibling sections between separate boundingBox protocol round trips.
      await expect.poll(() => home.evaluate(element => {
        const guide = document.querySelector('[data-testid="voiceprint-guide"]')?.getBoundingClientRect();
        const activity = document.querySelector('[data-testid="speech-activity"]')?.getBoundingClientRect();
        const timestamp = element.querySelector("time")?.getBoundingClientRect();
        if (!guide || !activity || !timestamp) return null;
        const guideGap = activity.top - guide.bottom;
        const transcriptGap = timestamp.top - activity.bottom;
        return { guideGap, transcriptGap, guideClear: guideGap >= 0,
          transcriptClear: transcriptGap >= 0, compactGap: transcriptGap < 48 };
      })).toMatchObject({ guideClear: true, transcriptClear: true, compactGap: true });
      const screenshot = testInfo.outputPath(`home-${language}-${theme}-${size}.png`);
      await page.screenshot({ path: screenshot, animations: "disabled" });
      await testInfo.attach(`home-${language}-${theme}-${size}`, {
        path: screenshot, contentType: "image/png"
      });
    }
    await app.close();
  });
}
