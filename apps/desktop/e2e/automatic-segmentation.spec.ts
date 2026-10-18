import type { DesktopSnapshot, RecordingHistoryItem } from "@daybee/shared";
import { expect, test } from "./fixtures";

const at = (seconds: number) => new Date(Date.parse("2026-10-09T10:00:00Z") + seconds * 1000).toISOString();
const chunk = (index: number, start = index * 8) => ({
  text: `第${index + 1}句，今天需要确认录音质量。`, voiceIdentity: "me", startedAt: at(start), endedAt: at(start + 4)
});

for (const role of ["compact"] as const) for (const theme of ["light", "dark"] as const) {
  test(`automatically segments long speech and reading paragraphs in ${role} ${theme}`, async ({ launchApp }, testInfo) => {
    const { app, page } = await launchApp();
    const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    snapshot.settings.desktop = { ...snapshot.settings.desktop, language: "zh-CN", theme };
    const speech: RecordingHistoryItem = {
      id: "long-speech", sessionId: "continuous-session", audioSource: "microphone", audioPath: "/recordings/long.flac",
      folderPath: "/recordings", inputDevice: null, status: "ok", startedAt: at(0),
      textPreview: "", transcriptChunks: Array.from({ length: 18 }, (_, index) => chunk(index))
    };
    snapshot.history = [speech];
    snapshot.microphoneActivity = { supported: true, error: null, items: [{
      id: "meeting", appId: "zoom", appName: "Zoom", appIcon: "", startedAt: at(-1), endedAt: at(300)
    }] };
    await app.setFixture(snapshot);
    if (role === "compact") await page.goto("/");
    await page.setViewportSize({ width: role === "compact" ? 1000 : 380, height: 1000 });
    await page.emulateMedia({ colorScheme: theme });
    const timeline = page.getByRole("list", { name: "转写时间线" });
    const nodes = timeline.getByRole("article");
    await expect(nodes).toHaveCount(3);
    await expect(nodes.locator(".transcript-speaker-name")).toHaveText(["我", "我", "我"]);
    await expect(nodes.locator(".recording-context")).toHaveText(["Zoom", "Zoom", "Zoom"]);
    expect(await nodes.evaluateAll(nodes => nodes.map(node => node.querySelectorAll(".transcript-paragraph").length))).toEqual([1, 2, 2]);
    expect(await nodes.evaluateAll(nodes => nodes.every(node => {
      const times = node.querySelectorAll("time");
      return Date.parse(times[1].dateTime) - Date.parse(times[0].dateTime) <= 60_000;
    }))).toBe(true);
    // Capture chunks flow inline inside a reading paragraph, rather than each
    // chunk forcing a fresh visual line.
    expect(await nodes.last().locator(".transcript-paragraph").first().evaluate(paragraph => {
      const spans = paragraph.querySelectorAll(".transcript-text");
      return spans[0].getClientRects()[0].top === spans[1].getClientRects()[0].top;
    })).toBe(true);
    await expect.poll(() => timeline.locator(".transcript-event").evaluateAll(rows => rows.every((row, index) => {
      const next = rows[index + 1];
      return !next || Math.abs(row.getBoundingClientRect().bottom - next.getBoundingClientRect().top) < 1;
    }))).toBe(true);
    await page.addStyleTag({ content: `html { background: ${theme === "dark" ? "#1c1c1e" : "#f5f5f7"}; }` });
    await page.screenshot({ path: testInfo.outputPath(`automatic-segmentation-${role}-${theme}.png`), animations: "disabled" });

    speech.transcriptChunks.push(chunk(18));
    await app.setFixture(snapshot);
    await expect(nodes).toHaveCount(3);
    await expect(nodes.first()).toContainText("第19句");
    const completed = await nodes.first().textContent();
    speech.transcriptChunks.push(chunk(19, 200));
    await app.setFixture(snapshot);
    await expect(nodes).toHaveCount(4);
    await expect(nodes.nth(1)).toHaveText(completed!);
    await expect(nodes.first()).toContainText("第20句");
    await app.close();
  });
}
