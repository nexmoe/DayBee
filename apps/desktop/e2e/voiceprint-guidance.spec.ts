import { expect, test } from "./fixtures";
import type { DesktopSnapshot } from "@daybee/shared";

for (const language of ["zh-CN", "en-US"] as const) {
  test(`makes the reading passage, completion and recovery clear in ${language}`, async ({ launchApp }) => {
    const { app, page } = await launchApp();
    await page.setViewportSize({ width: 1000, height: 800 });
    await page.goto("/?window=main");
    const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    snapshot.settings.desktop.language = language;
    snapshot.settings.desktop.voiceprintSetup = "ready";
    snapshot.voiceprint = {
      enrolled: true, phase: "recording", step: 1, targetSeconds: 8, speechSeconds: 2.4,
      level: .4, progress: null, error: null, message: "Raw backend status"
    };
    await app.setFixture(snapshot);
    const guide = page.getByTestId("voiceprint-guide");
    await expect(guide).toContainText(/第 2 段，共 3 段|Sample 2 of 3/);
    await expect(guide.getByRole("status")).toHaveText(/正在录制，请朗读以下文字|Recording · Read the text below aloud/);
    await expect(guide.getByTestId("voiceprint-passage")).toContainText(/安静的午后|On a quiet afternoon/);
    await expect(guide).toContainText(/停顿不会计入时长|pauses do not count/);
    await expect(guide).not.toContainText("Raw backend status");
    await expect(guide.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "2.4");
    await page.screenshot({ path: `e2e/results/voiceprint/reading-${language}.png`, animations: "disabled" });

    snapshot.voiceprint.phase = "sample-ready";
    snapshot.voiceprint.speechSeconds = 8;
    await app.setFixture(snapshot);
    await expect(guide.getByRole("status")).toHaveText(/这一段已录好|Sample recorded/);
    await expect(guide.getByRole("button", { name: /录制下一段|Record next sample/ })).toBeEnabled();
    await expect(guide).not.toContainText(/正在录制|Recording ·/);

    snapshot.voiceprint.step = 2;
    await app.setFixture(snapshot);
    await expect(guide.getByRole("button", { name: /开始验证|Start verification/ })).toBeEnabled();
    snapshot.voiceprint.phase = "recording";
    snapshot.voiceprint.step = 3;
    snapshot.voiceprint.targetSeconds = 4;
    snapshot.voiceprint.speechSeconds = 1;
    await app.setFixture(snapshot);
    await expect(guide).toContainText(/最后一步：验证声音|Final step · Verify your voice/);
    await expect(guide.getByTestId("voiceprint-passage")).toContainText(/这是我平时说话的声音|This is my usual speaking voice/);

    snapshot.voiceprint.phase = "error";
    snapshot.voiceprint.step = 1;
    snapshot.voiceprint.targetSeconds = 8;
    snapshot.voiceprint.error = "The speech sample was too short. Please try again.";
    await app.setFixture(snapshot);
    await expect(guide.getByRole("alert")).toContainText(/持续朗读至少 8 秒|at least 8 seconds/);
    await expect(guide.getByRole("alert")).toContainText(/之前保存的声纹仍可使用|previously saved voiceprint is still available/);
    await expect(page.getByText(snapshot.voiceprint.error, { exact: true })).toHaveCount(0);
    await expect(guide.getByRole("button")).toHaveCount(2);
    await expect(guide.getByRole("button", { name: /重新开始|Start over/ })).toBeEnabled();
    await expect(guide.getByRole("button", { name: /重新开始|Start over/ })).not.toHaveCSS("color", "rgba(0, 0, 0, 0)");
    await page.screenshot({ path: `e2e/results/voiceprint/error-${language}.png`, animations: "disabled" });
    await page.setViewportSize({ width: 480, height: 720 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await expect(guide.getByRole("alert")).toBeVisible();
    await expect(guide.getByRole("button", { name: /重新开始|Start over/ })).toBeVisible();
    await app.close();
  });
}
