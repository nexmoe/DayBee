import { readFileSync } from "node:fs";
import type { DesktopSnapshot, RecordingHistoryItem } from "@daybee/shared";
import { expect, test } from "./fixtures";

const icon = `data:image/png;base64,${readFileSync(new URL("../resources/icon.png", import.meta.url)).toString("base64")}`;
const recordedAt = Date.now() - 3600000;
const at = (seconds: number) => new Date(recordedAt + seconds * 1000).toISOString();
const recording = (id: string, chunks: [number, number, string][]): RecordingHistoryItem => ({
  id, audioSource: "microphone", audioPath: `/recordings/${id}.wav`, folderPath: "/recordings", startedAt: at(chunks[0][0]), status: "ok",
  inputDevice: null, sessionId: id, textPreview: chunks.map(chunk => chunk[2]).join("\n"),
  transcriptChunks: chunks.map(([start, end, text]) => ({ startedAt: at(start), endedAt: at(end), text }))
});

for (const language of ["zh-CN", "en-US"] as const) for (const theme of ["light", "dark"] as const) {
  test(`annotates transcripts with static recording context in ${language} ${theme}`, async ({ launchApp }, testInfo) => {
    const { app, page } = await launchApp();
    await page.emulateMedia({ colorScheme: theme });
    const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    snapshot.settings.desktop.language = language;
    snapshot.settings.desktop.theme = theme;
    snapshot.history = [
      recording("meeting", [[1, 4, "Personal note before meeting."], [12, 15, "Budget discussion and action items."], [31, 35, "Personal note after meeting."]]),
      recording("other", [[-600, -595, "Earlier conversation."]]),
      { ...recording("audio-only", [[16, 18, ""]]), status: "audio_only", transcriptChunks: [],
        capturedRanges: [{ startedAt: at(16), endedAt: at(18) }] }
    ];
    snapshot.microphoneActivity = { supported: true, error: null, items: [
      { id: "zoom", appId: "us.zoom.xos", appName: "Zoom", appIcon: icon, startedAt: at(10), endedAt: at(30) },
      { id: "teams", appId: "com.teams", appName: "Teams with a very long application name for narrow windows", appIcon: "", startedAt: at(-610), endedAt: at(-590) },
      { id: "unused", appId: "unused", appName: "Unused App", appIcon: "", startedAt: at(-1000), endedAt: at(-990) },
      { id: "audio-only-app", appId: "audio-only-app", appName: "Audio Only App", appIcon: "", startedAt: at(16), endedAt: at(18) },
      { id: "unidentified", appId: "pid:14559", appName: "pid:14559", appIcon: "", startedAt: at(10), endedAt: at(30) },
      { id: "live-unidentified", appId: "pid:34413", appName: "pid:34413", appIcon: "", startedAt: new Date(Date.now() - 10000).toISOString(), endedAt: null },
      { id: "live-zoom", appId: "us.zoom.xos", appName: "Zoom", appIcon: icon, startedAt: new Date(Date.now() - 10000).toISOString(), endedAt: null },
      { id: "live-zoom-device", appId: "us.zoom.xos", appName: "Zoom", appIcon: icon, startedAt: new Date(Date.now() - 5000).toISOString(), endedAt: null },
      { id: "live-teams", appId: "com.teams", appName: "Teams with a very long application name for narrow windows", appIcon: "", startedAt: new Date(Date.now() - 10000).toISOString(), endedAt: null }
    ] };
    await app.setFixture(snapshot);
    await expect(page.locator("html")).toHaveClass(theme === "dark" ? /dark/ : /^$/);
    await page.addStyleTag({ content: `html { background: ${theme === "dark" ? "#1c1c1e" : "#f5f5f7"}; }` });
    const compact = page.getByRole("region", { name: /Transcription timeline|转写时间线/ });
    const activeApps = page.getByRole("status", { name: /Using the microphone now|正在使用麦克风/ });
    await expect(activeApps.locator(".recording-context")).toHaveCount(2);
    await expect(activeApps).toContainText("Zoom");
    await expect(activeApps).not.toContainText("Unused App");
    await expect(activeApps.locator("img")).toHaveAttribute("src", icon);
    await expect(compact.getByRole("article")).toHaveCount(4);
    const meeting = compact.getByRole("article").filter({ hasText: "Budget discussion" });
    await expect(meeting.getByRole("complementary", { name: /Recording context|录音场景/ })).toContainText("Zoom");
    await expect(meeting.locator(".recording-context img")).toHaveAttribute("src", icon);
    expect(await meeting.locator("img").evaluate((image: HTMLImageElement) => image.complete && image.naturalWidth > 0)).toBe(true);
    await expect(compact.getByRole("article").filter({ hasText: "Personal note" }).getByRole("complementary")).toHaveCount(0);
    await expect(compact).not.toContainText("Unused App");
    expect(await compact.locator("time").evaluateAll(times => times.every(time =>
      time instanceof HTMLTimeElement && /^\d{2}:\d{2}:\d{2}$/.test(time.textContent!) &&
      time.title.includes(new Date(time.dateTime).getFullYear().toString())
    ))).toBe(true);
    const liveBounds = await activeApps.boundingBox();
    expect(liveBounds!.y + liveBounds!.height).toBeLessThan((await compact.locator("ol").boundingBox())!.y);
    await page.setViewportSize({ width: 380, height: 600 });
    expect(await compact.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true);
    await page.screenshot({ path: testInfo.outputPath(`context-${language}-${theme}-compact.png`), animations: "disabled" });

    await page.goto("/");
    await page.setViewportSize({ width: 1000, height: 760 });
    const home = page.getByRole("region", { name: /Home|首页/, exact: true });
    await expect(page.getByRole("combobox")).toHaveCount(0);
    await expect(page.getByRole("searchbox")).toHaveCount(0);
    await expect(home).not.toContainText("pid:");
    await page.addStyleTag({ content: `html { background: ${theme === "dark" ? "#1c1c1e" : "#f5f5f7"}; }` });
    await expect(home.getByRole("article")).toHaveCount(4);
    await expect(activeApps.locator(".recording-context")).toHaveCount(2);
    const homeMeeting = home.getByRole("article").filter({ hasText: "Budget discussion" });
    const annotation = await homeMeeting.getByRole("complementary").boundingBox();
    const text = await homeMeeting.locator(".transcript-prose").boundingBox();
    expect(annotation!.y + annotation!.height).toBeLessThanOrEqual(text!.y);
    expect(Math.abs(annotation!.x - text!.x)).toBeLessThan(1);
    await page.screenshot({ path: testInfo.outputPath(`context-${language}-${theme}-main.png`), animations: "disabled" });
    await app.setFixture({ ...snapshot, history: [] });
    await expect(home.getByRole("article")).toHaveCount(0);
    await expect(activeApps).toBeVisible();
    await expect(activeApps.getByRole("button")).toHaveCount(0);
    await app.setFixture(snapshot);
    await expect(home.getByRole("article")).toHaveCount(4);
    await expect(homeMeeting.getByRole("button")).toHaveCount(0);
    await expect(activeApps.getByRole("button")).toHaveCount(0);
    await page.getByRole("tablist").getByRole("tab", { name: /General|通用/, exact: true }).click();
    await page.getByRole("tablist").getByRole("tab", { name: /Home|首页/, exact: true }).click();
    await expect(home.getByRole("article")).toHaveCount(4);
    await expect(activeApps).toBeVisible();
    await app.setFixture({ ...snapshot, history: snapshot.history.filter(item => item.id === "other") });
    await expect(home.locator(".transcript-text")).toHaveText(["Earlier conversation."]);
    await app.setFixture(snapshot);
    await expect(home.getByRole("article")).toHaveCount(4);
    snapshot.microphoneActivity.items = snapshot.microphoneActivity.items.map(item => item.endedAt === null ?
      { ...item, endedAt: new Date().toISOString() } : item);
    await app.setFixture(snapshot);
    await expect(activeApps).toHaveCount(0);
    await expect(homeMeeting.getByRole("complementary")).toContainText("Zoom");
    snapshot.microphoneActivity.supported = false;
    await app.setFixture(snapshot);
    await expect(home.getByRole("status")).toContainText(/macOS 14.2/);
    await expect(home.getByRole("article")).toHaveCount(4);
    await app.close();
  });
}

test("merges continuous speech across an app start inside the first utterance in both windows", async ({ launchApp }, testInfo) => {
  const { app, page } = await launchApp();
  const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
  snapshot.history = [recording("continuous-scene", [[6.664, 12.814, "设置页允许配置是否录制系统声音。"],
    [13.608, 19.758, "然后系统声音和麦克风声音分开。"]])];
  snapshot.microphoneActivity = { supported: true, error: null, items: [{
    id: "continuous-wechat", appId: "com.tencent.inputmethod.wetype", appName: "微信输入法", appIcon: icon,
    startedAt: at(7.445), endedAt: at(22.445)
  }] };
  await app.setFixture(snapshot);
  for (const window of ["compact"]) {
    if (window === "compact") {
      await page.goto("/");
      await page.setViewportSize({ width: 1000, height: 760 });
    }
    const timeline = window === "compact" ? page.getByRole("region", { name: /Transcription timeline|转写时间线/ }) :
      page.getByRole("region", { name: /Home|首页/, exact: true });
    await expect(timeline.getByRole("article")).toHaveCount(1);
    await expect(timeline.locator(".transcript-text")).toHaveText(["设置页允许配置是否录制系统声音。", "然后系统声音和麦克风声音分开。"]);
    await expect(timeline.locator(".recording-context")).toHaveText("微信输入法");
    expect(await timeline.locator("time").evaluateAll(times => times.map(time => time.getAttribute("datetime"))))
      .toEqual([at(6.664), at(19.758)]);
    await page.screenshot({ path: testInfo.outputPath(`continuous-scene-${window}.png`), animations: "disabled" });
  }
  await app.close();
});
