import type { DesktopSnapshot, RecordingHistoryItem } from "@daybee/shared";
import { expect, test } from "./fixtures";

const start = Date.now() - 3_600_000;
const at = (seconds: number) => new Date(start + seconds * 1000).toISOString();
const recording = (id: string, from: number, to: number, text: string): RecordingHistoryItem => ({
  id, sessionId: id, audioSource: "microphone", audioPath: `/recordings/${id}.flac`, folderPath: "/recordings", inputDevice: null,
  startedAt: at(from), status: "ok", textPreview: text,
  transcriptChunks: [{ startedAt: at(from), endedAt: at(to), text }]
});

for (const role of ["compact"] as const) {
  test(`splits app and source changes into connected ${role} timeline nodes`, async ({ launchApp }, testInfo) => {
    const { app, page } = await launchApp();
    const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    const opening = "会议开场，确认今天需要讨论的问题。".repeat(10);
    snapshot.history = [recording("opening", 12, 18, opening), recording("dictation", 180, 190, "记录讨论方案。")];
    snapshot.microphoneActivity = { supported: true, error: null, items: [
      { id: "zoom-meeting", appId: "zoom", appName: "Zoom", appIcon: "", startedAt: at(10), endedAt: at(800) },
      { id: "dictation", appId: "wetype", appName: "微信输入法", appIcon: "", startedAt: at(175), endedAt: at(200) }
    ] };
    await app.setFixture(snapshot);
    if (role === "compact") {
      await page.goto("/");
      await page.setViewportSize({ width: 1000, height: 760 });
    }
    const timeline = page.getByRole("region", { name: role === "compact" ? /Home|首页/ : /Transcription timeline|转写时间线/, exact: true });
    await expect(timeline.getByRole("article")).toHaveCount(2);
    await expect(timeline.locator(".transcript-turn")).toHaveCount(2);
    await expect(timeline.locator(".recording-context")).toHaveText(["微信输入法", "Zoom", "Zoom"]);
    expect((await timeline.locator(".transcript-text").allTextContents()).join("").replace(/\s/gu, "")).toBe(`记录讨论方案。${opening}`);
    // The secondary app ending creates a fresh node for the resumed meeting.
    snapshot.history.push(recording("later-session", 720, 730, "会议确认下一步。"));
    await app.setFixture(snapshot);
    await expect(timeline.getByRole("article")).toHaveCount(3);
    await expect(timeline.getByRole("article").first()).toContainText("会议确认下一步。");
    expect(await timeline.locator("time").evaluateAll(times => times.map(time => time.getAttribute("datetime"))))
      .toEqual([at(720), at(730), at(180), at(190), at(12), at(18)]);
    await page.screenshot({ path: testInfo.outputPath(`app-segment-${role}.png`), animations: "disabled" });

    snapshot.history.push({ ...recording("system-playback", 400, 410, "单独保存的系统声音。"), audioSource: "system" });
    await app.setFixture(snapshot);
    await expect(timeline.getByRole("article")).toHaveCount(4);
    await expect(timeline.locator('[data-audio-source="microphone"]')).toHaveCount(3);
    await expect(timeline.getByRole("article").first()).toContainText("会议确认下一步。");
    await expect(timeline.locator('[data-audio-source="system"] .recording-context')).toHaveCount(0);

    snapshot.history.push(recording("next-meeting", 910, 920, "第二次会议。"));
    snapshot.microphoneActivity.items.push({
      id: "next-zoom-meeting", appId: "zoom", appName: "Zoom", appIcon: "", startedAt: at(900), endedAt: at(950)
    });
    await app.setFixture(snapshot);
    await expect(timeline.getByRole("article")).toHaveCount(5);
    await expect(timeline.getByRole("article").first()).toContainText("第二次会议。");
    await app.close();
  });
}
