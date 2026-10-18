import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { DesktopSnapshot } from "@daybee/shared";
import { expect, test } from "./fixtures";

test("system audio capture is opt-in and persists through relaunch", async ({ launchApp }) => {
  test.skip(process.platform !== "darwin", "System audio capture currently supports macOS");
  let { app, page } = await launchApp();
  const openInput = async () => {
    await page.goto("/?window=main");
    await page.setViewportSize({ width: 1000, height: 760 });
    await page.getByRole("navigation").getByRole("button", { name: /Input|输入/, exact: true }).click();
  };
  await openInput();
  let toggle = page.getByTestId("system-audio-field").getByRole("switch");
  await expect(toggle).not.toBeChecked();
  await toggle.click();
  await expect.poll(async () => (await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"))).settings.recording.recordSystemAudio).toBe(true);
  await app.close();
  ({ app, page } = await launchApp());
  await openInput();
  toggle = page.getByTestId("system-audio-field").getByRole("switch");
  await expect(toggle).toBeChecked();
  await page.setViewportSize({ width: 380, height: 640 });
  await expect(toggle).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await toggle.click();
  await expect.poll(async () => (await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"))).settings.recording.recordSystemAudio).toBe(false);
  await app.close();
});

test("saved microphone and system speech have distinct timeline labels and history counts", async ({ launchApp }, testInfo) => {
  const { app, page } = await launchApp();
  const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
  const directory = snapshot.settings.recording.outputDir;
  mkdirSync(directory, { recursive: true });
  for (const source of ["microphone", "system"] as const) {
    writeFileSync(join(directory, `${source}.json`), JSON.stringify({
      audio_path: join(directory, `${source}.wav`), audio_source: source,
      segment_start_time: "2026-10-09T14:00:00+08:00", recording_session_id: "same-session",
      status: "ok", text: source === "system" ? "视频中的一句话。" : "麦克风里的一句话。"
    }));
  }
  await app.refreshHistory();
  for (const role of ["compact", "main"] as const) {
    await page.goto(role === "main" ? "/?window=main" : "/");
    await page.setViewportSize({ width: role === "main" ? 1000 : 380, height: 760 });
    const mic = page.locator('.transcript-event[data-audio-source="microphone"]');
    const system = page.locator('.transcript-event[data-audio-source="system"]');
    await expect(mic.locator(".transcript-text")).toHaveText("麦克风里的一句话。");
    await expect(system.locator(".transcript-text")).toHaveText("视频中的一句话。");
    await expect(mic.getByTestId("audio-source-label")).toHaveText(/Microphone|麦克风/);
    await expect(system.getByTestId("audio-source-label")).toHaveText(/System audio|系统声音/);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    if (role === "main") {
      await page.screenshot({ path: testInfo.outputPath("audio-source-timeline.png"), animations: "disabled" });
    }
  }
  await page.getByRole("navigation").getByRole("button", { name: /History|历史/, exact: true }).click();
  await expect(page.getByTestId("history-source-summary")).toHaveText(/(?:Microphone|麦克风) 1 · (?:System audio|系统声音) 1/);
  await expect.poll(async () => (await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"))).history.map(item => item.audioSource).sort()).toEqual(["microphone", "system"]);
  await app.close();
});
