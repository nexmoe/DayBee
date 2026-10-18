import type { DesktopSnapshot, RecordingHistoryItem } from "@daybee/shared";
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "./fixtures";

async function openMain(page: import("@playwright/test").Page) {
  await page.goto("/?window=main");
  await page.setViewportSize({ width: 1000, height: 760 });
}

async function bootstrap(page: import("@playwright/test").Page) {
  return page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
}

test("keeps the home voiceprint card after skipping, canceling and relaunching", async ({ launchApp }) => {
  let context = await launchApp({ voiceprintSetup: "pending" });
  await context.page.getByRole("button", { name: /Start recording|开始录音/ }).click();
  await expect.poll(async () => (await bootstrap(context.page)).status.recording).toBe(false);
  await openMain(context.page);
  const guide = context.page.getByTestId("voiceprint-guide");
  await expect(guide).toBeVisible();
  await expect(guide).toContainText(/录制 3 段各 8 秒|three 8-second samples/);
  await context.page.screenshot({ path: "e2e/results/voiceprint/intro.png", animations: "disabled" });
  await guide.getByRole("button", { name: /暂时跳过|Skip for now/ }).click();
  await expect(guide).toBeVisible();
  await expect(guide.getByRole("button", { name: /^开始录制$|^Start recording$/ })).toBeVisible();
  await expect.poll(async () => (await bootstrap(context.page)).settings.desktop.voiceprintSetup).toBe("skipped");
  await expect.poll(async () => (await bootstrap(context.page)).voiceprint.enrolled).toBe(false);
  await expect.poll(async () => (await bootstrap(context.page)).status.recording).toBe(true);
  await context.app.close();

  context = await launchApp();
  await openMain(context.page);
  const relaunchedGuide = context.page.getByTestId("voiceprint-guide");
  await expect(relaunchedGuide).toBeVisible();
  await relaunchedGuide.getByRole("button", { name: /取消|Cancel/ }).click();
  await expect(relaunchedGuide).toBeVisible();
  await expect(relaunchedGuide.getByRole("button", { name: /^开始录制$|^Start recording$/ })).toBeVisible();
  await context.page.getByRole("navigation").getByRole("button", { name: /History|历史/, exact: true }).click();
  await context.page.getByRole("navigation").getByRole("button", { name: /Home|首页/, exact: true }).click();
  await expect(relaunchedGuide).toBeVisible();
  await context.page.reload();
  await expect(relaunchedGuide).toBeVisible();
  await context.app.close();
});

test("uses enrolled voiceprints automatically across relaunch and supports record again or clear", async ({ launchApp, storeDir }) => {
  const { app, page } = await launchApp({ voiceprintSetup: "pending" });
  await openMain(page);
  const guide = page.getByTestId("voiceprint-guide");
  await guide.getByRole("button", { name: /^开始录制$|^Start recording$/ }).click();
  await expect.poll(async () => (await bootstrap(page)).voiceprint.phase, { timeout: 20000 }).toBe("sample-ready");
  await expect(guide.getByTestId("voiceprint-passage")).toContainText(/今天我用平常的声音|Today I am speaking/);
  await expect(guide.getByRole("progressbar", { name: /有效人声|Valid speech/ })).toBeVisible();
  await page.screenshot({ path: "e2e/results/voiceprint/sample.png", animations: "disabled" });
  await guide.getByRole("button", { name: /录制下一段|Record next sample/ }).click();
  await expect.poll(async () => (await bootstrap(page)).voiceprint.step).toBe(1);
  await expect.poll(async () => (await bootstrap(page)).voiceprint.phase, { timeout: 20000 }).toBe("sample-ready");
  await expect(guide.getByTestId("voiceprint-passage")).toContainText(/安静的午后|On a quiet afternoon/);
  await guide.getByRole("button", { name: /录制下一段|Record next sample/ }).click();
  await expect.poll(async () => (await bootstrap(page)).voiceprint.step).toBe(2);
  await expect.poll(async () => (await bootstrap(page)).voiceprint.phase, { timeout: 20000 }).toBe("sample-ready");
  await expect(guide.getByTestId("voiceprint-passage")).toContainText(/一段有用的录音|A useful recording/);
  await guide.getByRole("button", { name: /开始验证|Start verification/ }).click();
  await expect.poll(async () => (await bootstrap(page)).voiceprint.step).toBe(3);
  await expect.poll(async () => (await bootstrap(page)).voiceprint.phase, { timeout: 20000 }).toBe("ready");
  await expect.poll(async () => (await bootstrap(page)).settings.desktop.voiceprintSetup).toBe("ready");
  await expect.poll(async () => (await bootstrap(page)).voiceprint.enrolled).toBe(true);
  await expect(guide).toBeVisible();
  await expect(guide).toContainText(/转写时会将你的发言|marked as Me in transcripts/);

  await page.getByRole("navigation").getByRole("button", { name: /Transcribe|转写/, exact: true }).click();
  await expect(page.getByTestId("voiceprint-enabled-field")).toHaveCount(0);
  await expect(page.getByTestId("voiceprint-manage-field")).toContainText(/转写时会将你的发言|marked as Me in transcripts/);
  await page.getByRole("button", { name: /重新录制|Record again/ }).click();
  await expect(page.getByTestId("voiceprint-guide")).toBeVisible();
  await page.getByTestId("voiceprint-guide").getByRole("button", { name: /取消|Cancel/ }).click();
  await expect(guide).toBeVisible();
  await expect(guide.getByRole("button", { name: /重新录制|Record again/ })).toBeVisible();
  await guide.getByRole("button", { name: /重新录制|Record again/ }).click();
  await expect.poll(async () => (await bootstrap(page)).voiceprint.phase).toBe("sample-ready");
  await guide.getByRole("button", { name: /取消|Cancel/ }).click();
  await expect.poll(async () => (await bootstrap(page)).voiceprint.phase).toBe("idle");
  await expect(guide).toBeVisible();
  await expect(guide.getByRole("button", { name: /重新录制|Record again/ })).toBeVisible();
  await expect.poll(async () => (await bootstrap(page)).voiceprint.enrolled).toBe(true);
  await page.screenshot({ path: "e2e/results/voiceprint/canceled.png", animations: "disabled" });
  await app.close();

  // An older disabled preference must not prevent automatic use after migration.
  const settingsPath = join(storeDir, "desktop-settings.json");
  const saved = JSON.parse(readFileSync(settingsPath, "utf8"));
  saved.settings.recording.voiceprintEnabled = false;
  writeFileSync(settingsPath, JSON.stringify(saved));
  const relaunched = await launchApp();
  await openMain(relaunched.page);
  await expect(relaunched.page.getByTestId("voiceprint-guide")).toBeVisible();
  expect((await bootstrap(relaunched.page)).settings.recording).not.toHaveProperty("voiceprintEnabled");
  await expect.poll(async () => (await bootstrap(relaunched.page)).voiceprint.enrolled).toBe(true);
  await relaunched.page.getByRole("navigation").getByRole("button", { name: /Transcribe|转写/, exact: true }).click();
  await expect(relaunched.page.getByTestId("voiceprint-manage-field")).toContainText(/转写时会将你的发言|marked as Me in transcripts/);
  await expect(relaunched.page.getByTestId("voiceprint-enabled-field")).toHaveCount(0);
  await relaunched.page.screenshot({ path: "e2e/results/voiceprint/settings.png", animations: "disabled" });
  await relaunched.page.getByRole("button", { name: /清除声纹|Clear voiceprint/ }).click();
  await expect.poll(async () => (await bootstrap(relaunched.page)).voiceprint.enrolled).toBe(false);
  await expect(relaunched.page.getByRole("button", { name: /录入我的声音|Set up my voice/, exact: true })).toBeVisible();
  await relaunched.app.close();
});

test("labels matching sentences and keeps every transcript line", async ({ launchApp }) => {
  const { app, page } = await launchApp();
  await openMain(page);
  const snapshot = await bootstrap(page);
  const at = (seconds: number) => new Date(Date.parse("2026-10-09T11:00:00Z") + seconds * 1000).toISOString();
  const item = (id: string, audioSource: "microphone" | "system", chunks: RecordingHistoryItem["transcriptChunks"]): RecordingHistoryItem => ({
    id, audioSource, audioPath: `/recordings/${id}.flac`, folderPath: "/recordings", inputDevice: null, status: "ok",
    startedAt: chunks[0].startedAt, sessionId: "scene", textPreview: chunks.map(chunk => chunk.text).join("\n"), transcriptChunks: chunks
  });
  snapshot.history = [
    item("mic", "microphone", [
      { text: "Hello from me. Still me.", startedAt: at(0), endedAt: at(2), voiceIdentity: "me" },
      { text: "Someone else.", startedAt: at(3), endedAt: at(5) },
      { text: "Back again.", startedAt: at(6), endedAt: at(8), voiceIdentity: "me" }
    ]),
    item("system", "system", [
      { text: "System playback.", startedAt: at(1), endedAt: at(3), voiceIdentity: "me" }
    ]),
    item("legacy", "microphone", [
      { text: "Legacy line.", startedAt: at(3600), endedAt: at(3602) }
    ])
  ];
  await app.setFixture(snapshot);
  const home = page.getByRole("region", { name: /Home|首页/, exact: true });
  await expect(home.locator(".transcript-text")).toHaveText([
    "Legacy line.", "Back again.", "Someone else.", "System playback.", "Hello from me.", "Still me."
  ]);
  await expect(home.locator(".transcript-speaker-name")).toHaveText([
    "Identity unconfirmed", "Me", "Identity unconfirmed", "Identity unconfirmed", "Me"
  ]);
  await expect(home.getByRole("article").filter({ hasText: "System playback." }).getByTestId("audio-source-label")).toHaveText("System audio");
  await expect(home.getByRole("article").filter({ hasText: "System playback." }).locator('[data-voice-identity="me"]')).toHaveCount(0);
  await page.screenshot({ path: "e2e/results/voiceprint/identities.png", animations: "disabled" });
  expect(await home.locator(".transcript-text").allTextContents()).not.toEqual(expect.arrayContaining([expect.stringMatching(/^我$|^Me$/)]));
  await page.getByRole("searchbox").fill("Someone else");
  await expect(home.locator(".transcript-text")).toHaveText(["Someone else."]);
  await page.getByRole("searchbox").fill("我");
  await expect(home.getByRole("article")).toHaveCount(0);
  await app.close();
});

for (const role of ["compact", "main"] as const) for (const language of ["zh-CN", "en-US"] as const) {
  test(`groups continuous speech under one speaker heading in ${role} ${language}`, async ({ launchApp }, testInfo) => {
    const { app, page } = await launchApp();
    const snapshot = await bootstrap(page);
    await page.evaluate(async language => {
      const current = await window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap");
      await window.mygo!.call("DesktopService.SaveSettings", {
        ...current.settings, desktop: { ...current.settings.desktop, language }
      });
    }, language);
    snapshot.settings.desktop.language = language;
    snapshot.history = [{
      id: "sentences", audioSource: "microphone", audioPath: "/recordings/sentences.flac", folderPath: "/recordings",
      inputDevice: null, status: "ok", startedAt: "2026-10-09T11:00:00Z", sessionId: "scene",
      textPreview: "你好，测试一下。你是谁啊？你好，你好。\n未确认身份。还有一句。",
      transcriptChunks: [
        { text: "你好，测试一下。你是谁啊？你好，你好。", voiceIdentity: "me", startedAt: "2026-10-09T11:00:00Z", endedAt: "2026-10-09T11:00:06Z" },
        { text: "未确认身份。还有一句。", startedAt: "2026-10-09T11:00:07Z", endedAt: "2026-10-09T11:00:10Z" }
      ]
    }];
    await app.setFixture(snapshot);
    if (role === "main") await openMain(page);
    else await page.setViewportSize({ width: 380, height: 720 });
    const rows = page.locator(".transcript-turn");
    await expect(rows.locator(".transcript-text")).toHaveText(["未确认身份。", "还有一句。", "你好，测试一下。", "你是谁啊？", "你好，你好。"]);
    const me = language === "zh-CN" ? "我" : "Me";
    const unknown = language === "zh-CN" ? "身份未确认" : "Identity unconfirmed";
    await expect(rows.locator(".transcript-speaker-name")).toHaveText([unknown, me]);
    expect(await rows.evaluateAll(elements => elements.every(element => {
      const heading = element.querySelector(".transcript-entry-header")!.getBoundingClientRect();
      const text = element.querySelector(".transcript-prose")!.getBoundingClientRect();
      return heading.left === text.left && heading.bottom <= text.top && text.right <= innerWidth;
    }))).toBe(true);
    await expect(page.locator('.transcript-turn[data-voice-identity="unknown"]')).toHaveCount(1);
    expect(await rows.locator(".transcript-entry-header").last().locator("time").evaluateAll(times => times.map(time => time.getAttribute("datetime"))))
      .toEqual(["2026-10-09T11:00:00.000Z", "2026-10-09T11:00:06.000Z"]);
    await page.screenshot({ path: `e2e/results/voiceprint/speaker-sentences-${role}-${language}.png`, animations: "disabled" });
    await app.close();
  });
}
