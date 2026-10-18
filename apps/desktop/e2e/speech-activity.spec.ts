import type { DesktopSnapshot, RecordingHistoryItem, TranscriptChunk } from "@daybee/shared";
import { expect, test } from "./fixtures";

test.use({ timezoneId: "Asia/Shanghai" });

const speech = (text: string, startedAt: string, voiceIdentity?: string, voiceMatch?: string): TranscriptChunk => ({
  text, startedAt, endedAt: new Date(Date.parse(startedAt) + 5000).toISOString(), voiceIdentity, voiceMatch
});
const record = (id: string, chunks: TranscriptChunk[], audioSource: "microphone" | "system" = "microphone"): RecordingHistoryItem => ({
  id, sessionId: id, audioPath: `/recordings/${id}.flac`, folderPath: "/recordings", inputDevice: null, audioSource,
  startedAt: chunks[0].startedAt, status: "ok", textPreview: chunks.map(chunk => chunk.text).join("\n"), transcriptChunks: chunks
});

function speechFixture(snapshot: DesktopSnapshot) {
  snapshot.settings.desktop.language = "zh-CN";
  snapshot.history = [
    record("mixed", [speech("你好！", "2026-10-08T10:00:00+08:00", "me"),
      speech("大家好。", "2026-10-08T10:05:00+08:00"), speech("我也来了。", "2026-10-08T10:10:00+08:00", undefined, "partial")]),
    record("overnight", [speech("前一天的发言。", "2026-10-07T23:59:00+08:00", "me"), speech("再见。", "2026-10-08T00:01:00+08:00", "me")]),
    record("system", [speech("电脑播放的内容。", "2026-10-08T11:00:00+08:00", "me")], "system"),
    record("next-day", [speech("另一天的发言。", "2026-10-09T11:00:00+08:00", "me")])
  ];
  snapshot.microphoneActivity = { supported: true, error: null, items: [{
    id: "meeting", appId: "zoom", appName: "Zoom", appIcon: "",
    startedAt: "2026-10-08T10:00:00+08:00", endedAt: "2026-10-08T10:30:00+08:00"
  }] };
  return snapshot;
}

test("selects dates and keeps speaker totals and transcripts live", async ({ launchApp }) => {
  const { app, page } = await launchApp();
  const snapshot = speechFixture(await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap")));
  await app.setFixture(snapshot);
  await page.clock.setFixedTime(new Date("2026-10-08T12:00:00+08:00"));
  await page.goto("/");
  await page.setViewportSize({ width: 1000, height: 900 });
  const chart = page.getByTestId("speech-activity");
  const home = page.getByRole("region", { name: "首页", exact: true });
  const calendar = chart.getByLabel("选择日期", { exact: true });
  await calendar.fill("2026-10-08");
  await expect(chart.locator('[data-speech-date="2026-10-08"]')).toBeChecked();
  await expect(chart.getByTestId("speech-count-me")).toHaveText("8");
  await expect(chart.getByTestId("speech-count-other")).toHaveText("3");
  await expect(chart.getByTestId("speech-activity-total")).toHaveText("11 字");
  // Enter the legends with the keyboard so the tooltip gets visible focus.
  await chart.getByLabel("我 · 8 字", { exact: true }).focus();
  await page.keyboard.press("Shift+Tab");
  await page.keyboard.press("Tab");
  await expect(chart.getByLabel("我 · 8 字", { exact: true })).toBeFocused();
  await expect(page.locator(".speech-chart-tooltip").filter({ hasText: "我 · 8 字" })).toBeVisible();
  await page.keyboard.press("Tab");
  await expect(chart.getByLabel("其他 · 3 字", { exact: true })).toBeFocused();
  await expect(page.locator(".speech-chart-tooltip").filter({ hasText: "其他 · 3 字" })).toBeVisible();
  await expect(home).toContainText("再见。");
  await expect(home).toContainText("电脑播放的内容。");
  await expect(home).not.toContainText("前一天的发言。");
  await expect(home).not.toContainText("另一天的发言。");
  await expect(chart.locator(".speech-stacked-bar")).toHaveCount(7);

  snapshot.history[0].transcriptChunks.push(speech("补充内容。", "2026-10-08T10:20:00+08:00", "me"));
  await app.setFixture(snapshot);
  await expect(chart.getByTestId("speech-count-me")).toHaveText("12");
  await expect(chart.getByTestId("speech-activity-total")).toHaveText("15 字");
  await expect(chart.locator('[data-speech-date="2026-10-08"]')).toBeChecked();
  await expect(home).toContainText("补充内容。");
  await expect(home).toContainText("电脑播放的内容。");
  await expect(home).toContainText("再见。");

  await chart.locator('[data-speech-date="2026-10-08"]').focus();
  await page.keyboard.press("ArrowLeft");
  await expect(chart.locator('[data-speech-date="2026-10-07"]')).toBeChecked();
  await expect(home).toContainText("前一天的发言。");
  await page.keyboard.press("Home");
  await expect(chart.locator('[data-speech-date="2026-10-02"]')).toBeChecked();
  await expect(home.getByRole("heading", { name: "当天还没有转写" })).toBeVisible();
  await calendar.fill("2026-10-08");
  await chart.getByRole("button", { name: "上一周" }).click();
  await expect(chart.locator('[data-speech-date="2026-10-01"]')).toBeChecked();
  await chart.getByRole("button", { name: "下一周" }).click();
  await expect(chart.locator('[data-speech-date="2026-10-08"]')).toBeChecked();
  await expect(chart.locator(".speech-stacked-bar")).toHaveCount(7);
  await chart.getByRole("button", { name: "今天", exact: true }).click();
  await expect(chart.getByRole("button", { name: "下一周" })).toBeDisabled();
  const today = await page.evaluate(() => {
    const now = new Date();
    return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;
  });
  expect(await calendar.getAttribute("max")).toBe(today);
  await expect(chart.locator(`[data-speech-date="${today}"]`)).toBeChecked();
  expect(await chart.getByRole("radio").evaluateAll(days => days.map(day => day.getAttribute("data-speech-date"))))
    .toEqual(["2026-10-02", "2026-10-03", "2026-10-04", "2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08"]);
  await app.close();
});

test("shows the same date chart and filters transcripts in the compact recorder", async ({ launchApp }) => {
  const { app, page } = await launchApp();
  const snapshot = speechFixture(await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap")));
  await app.setFixture(snapshot);
  await page.clock.setFixedTime(new Date("2026-10-08T12:00:00+08:00"));
  await page.reload();
  const chart = page.getByTestId("speech-activity");
  const timeline = page.getByRole("list", { name: "转写时间线", exact: true });
  await expect(page.locator('[data-window="compact"]')).toBeVisible();
  await expect(chart).toHaveCount(1);
  await expect(chart.getByRole("radio")).toHaveCount(7);
  await chart.getByLabel("选择日期", { exact: true }).fill("2026-10-08");
  await expect(chart.getByTestId("speech-count-me")).toHaveText("8");
  await expect(chart.getByTestId("speech-count-other")).toHaveText("3");
  await expect(timeline).toContainText("再见。");
  await expect(timeline).not.toContainText("前一天的发言。");
  await chart.locator('[data-speech-date="2026-10-08"]').press("ArrowLeft");
  await expect(chart.locator('[data-speech-date="2026-10-07"]')).toBeChecked();
  await expect(timeline).toContainText("前一天的发言。");
  await expect(timeline).not.toContainText("再见。");
  await chart.getByRole("button", { name: "今天", exact: true }).click();
  await expect(chart.locator('[data-speech-date="2026-10-08"]')).toBeChecked();
  await expect(timeline).toContainText("再见。");
  await expect(timeline).not.toContainText("前一天的发言。");
  await app.close();
});

for (const role of ["compact"] as const) for (const language of ["zh-CN", "en-US"] as const) for (const theme of ["light", "dark"] as const) {
  test(`renders one two-color daily chart in ${role} ${language} ${theme}`, async ({ launchApp }, testInfo) => {
    const { app, page } = await launchApp();
    const snapshot = speechFixture(await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap")));
    snapshot.settings.desktop = { ...snapshot.settings.desktop, language, theme };
    snapshot.voiceprint.enrolled = true;
    snapshot.history = Array.from({ length: 7 }, (_, index) => {
      const date = `2026-10-${String(index + 5).padStart(2, "0")}`;
      return record(`day-${index}`, [
        speech("今天一起讨论新的计划。".repeat(index + 2), `${date}T09:00:00+08:00`, "me"),
        speech("还有一些想法需要补充。".repeat(8 - index), `${date}T09:05:00+08:00`),
        speech("下午确认进展。".repeat(index + 1), `${date}T14:00:00+08:00`, "me"),
        speech("最后整理会议记录。".repeat(4), `${date}T18:00:00+08:00`)
      ]);
    });
    snapshot.history[3] = record("selected-day", [
      speech("今天一起讨论新的计划".repeat(437).slice(0, 4366), "2026-10-08T09:00:00+08:00", "me"),
      speech("还有一些想法需要补充".repeat(10081).slice(0, 100801), "2026-10-08T09:05:00+08:00")
    ]);
    await app.setFixture(snapshot);
    await page.clock.setFixedTime(new Date("2026-10-13T12:00:00+08:00"));
    await page.goto(role === "compact" ? "/" : "/");
    await page.emulateMedia({ colorScheme: theme, reducedMotion: "reduce" });
    await page.addStyleTag({ content: `html { background: ${theme === "dark" ? "#1c1c1e" : "#f5f5f7"}; }` });
    const chart = page.getByTestId("speech-activity");
    await expect(chart.getByRole("button", { name: language === "zh-CN" ? "全部日期" : "All dates", exact: true })).toHaveCount(0);
    await expect(chart).not.toContainText(language === "zh-CN" ? "按麦克风转写统计，不含标点和空格" : "Microphone transcripts · excludes punctuation and spaces");
    await expect(chart.getByRole("radio")).toHaveCount(7);
    await expect(chart.locator(".speech-stacked-bar")).toHaveCount(7);
    await expect(chart.locator('[data-speech-date="2026-10-14"]')).toHaveCount(0);
    const sizes = role === "compact" ? [[1000, 900], [720, 900], [380, 900]] : [[420, 620], [380, 480]];
    for (const [width, height] of sizes) {
      await page.setViewportSize({ width, height });
      await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      expect(await chart.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
      const daily = chart.locator('[data-speech-date="2026-10-08"]');
      const colors = await daily.locator(".speech-stacked-bar > span").evaluateAll(bars => bars.map(bar => getComputedStyle(bar).backgroundColor));
      expect(colors).toHaveLength(2);
      expect(colors[0]).not.toBe(colors[1]);
      expect(await chart.locator(".speech-stacked-bar").evaluateAll(bars => bars.every(bar => bar.getBoundingClientRect().height > 0))).toBe(true);
      await page.evaluate(() => document.fonts.ready);
      const geometry = () => chart.getByRole("radio").evaluateAll(days => days.map(day => {
        const origin = day.parentElement!.getBoundingClientRect();
        const bounds = (element: Element) => {
          const rect = element.getBoundingClientRect();
          return [rect.x - origin.x, rect.y - origin.y, rect.width, rect.height];
        };
        const style = getComputedStyle(day);
        const box = day.getBoundingClientRect();
        return {
          date: day.getAttribute("data-speech-date"),
          bounds: bounds(day),
          content: Array.from(day.querySelectorAll(".speech-date-label, .speech-stacked-bar, .speech-day-count"), bounds),
          padding: [style.paddingTop, style.paddingRight, style.paddingBottom, style.paddingLeft],
          textInsets: Array.from(day.querySelectorAll(".speech-date-label > span, .speech-day-count"), text => {
            const range = document.createRange();
            range.selectNodeContents(text);
            const rect = range.getBoundingClientRect();
            return [rect.left - box.left, box.right - rect.right];
          })
        };
      }));
      const before = await geometry();
      for (const day of before) {
        expect(day.textInsets.flat().every(inset => inset >= 7.5), `Text needs room on both sides: ${JSON.stringify(day)}`).toBe(true);
      }
      for (const date of ["2026-10-07", "2026-10-08"]) {
        const target = chart.locator(`[data-speech-date="${date}"]`);
        await target.click();
        await expect(target).toBeChecked();
        expect(await geometry()).toEqual(before);
      }
      await page.mouse.move(0, 0);
      await expect(chart.locator('[data-slot="fluid-hover-highlight"]')).toHaveCount(0);
      const screenshot = testInfo.outputPath(`speech-${role}-${language}-${theme}-${width}.png`);
      await page.screenshot({ path: screenshot, animations: "disabled" });
      await testInfo.attach(`speech-${width}`, { path: screenshot, contentType: "image/png" });
    }
    await app.close();
  });
}
