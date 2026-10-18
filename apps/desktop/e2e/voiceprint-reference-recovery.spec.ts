import type { DesktopSnapshot } from "@daybee/shared";
import { expect, test } from "./fixtures";

for (const language of ["zh-CN", "en-US"] as const) {
  test(`explains paused learning and offers playback-check recovery in ${language}`, async ({ launchApp }) => {
    const { app, page } = await launchApp({ voiceprintSetup: "ready" });
    await page.goto("/?window=main");
    await page.setViewportSize({ width: 1000, height: 900 });
    const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    snapshot.settings.desktop.language = language;
    snapshot.settings.recording.voiceprintAdaptation = true;
    snapshot.settings.recording.disableAsr = false;
    snapshot.voiceprint.enrolled = true;
    snapshot.voiceprint.phase = "ready";
    snapshot.voiceprint.adaptation = { learnedClips: 0, assistedClips: 0 };
    snapshot.status.recording = true;
    snapshot.status.asrEnabled = true;
    snapshot.status.voiceprintReference = "unavailable";
    snapshot.status.voiceprintReferenceError = "System audio capture was stopped by the user.";
    snapshot.status.voiceprintContextMismatch = false;
    await app.setFixture(snapshot);
    await page.getByRole("navigation").getByRole("button", { name: /Transcribe|转写/, exact: true }).click();
    await expect(page.getByTestId("voiceprint-advanced-toggle")).toContainText(language === "zh-CN" ? "持续适应已暂停" : "Adaptation paused");
    await page.getByTestId("voiceprint-advanced-toggle").click();
    const field = page.getByTestId("voiceprint-adaptation-field");
    await expect(field).toContainText(language === "zh-CN" ? "学习已暂停" : "Learning is paused");
    await expect(field).not.toContainText(language === "zh-CN" ? "恢复后继续学习" : "before learning");
    const settings = field.getByTestId("voiceprint-reference-settings");
    const retry = field.getByTestId("voiceprint-reference-retry");
    await expect(settings).toBeVisible();
    await expect(retry).toBeEnabled();
    const detail = field.getByTestId("voiceprint-reference-error");
    await expect(detail).not.toHaveAttribute("open");
    await detail.locator("summary").click();
    await expect(detail).toContainText(snapshot.status.voiceprintReferenceError);
    await detail.locator("summary").click();

    await page.evaluate((next) => {
      const original = window.mygo!.call.bind(window.mygo);
      const host = window as unknown as { recoveryCalls: string[] };
      host.recoveryCalls = [];
      window.mygo!.call = ((method: string, ...args: unknown[]) => {
        if (method === "DesktopService.OpenSystemAudioSettings") {
          host.recoveryCalls.push(method);
          return Promise.resolve(true);
        }
        if (method === "DesktopService.RetryVoiceprintReference") {
          host.recoveryCalls.push(method);
          return original("DesktopService.Bootstrap").then(() => next);
        }
        return original(method, ...args);
      }) as NonNullable<Window["mygo"]>["call"];
    }, snapshot);
    await settings.click();
    await retry.click();
    expect(await page.evaluate(() => (window as unknown as { recoveryCalls: string[] }).recoveryCalls)).toEqual([
      "DesktopService.OpenSystemAudioSettings", "DesktopService.RetryVoiceprintReference"
    ]);
    await expect(page.getByTestId("voiceprint-adaptation-learned")).toContainText("0");
    await expect(page.getByTestId("voiceprint-adaptation-assisted")).toContainText("0");
    await field.screenshot({ path: `e2e/results/voiceprint/paused-reference-${language}.png`, animations: "disabled" });
    await page.setViewportSize({ width: 480, height: 900 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await field.screenshot({ path: `e2e/results/voiceprint/paused-reference-narrow-${language}.png`, animations: "disabled" });

    snapshot.status.voiceprintReference = "starting";
    await app.setFixture(snapshot);
    await expect(retry).not.toBeVisible();
    snapshot.status.voiceprintReference = "available";
    snapshot.status.voiceprintReferenceError = "";
    await app.setFixture(snapshot);
    await expect(field).not.toContainText(language === "zh-CN" ? "学习已暂停" : "Learning is paused");
    await expect(settings).not.toBeVisible();
    await expect(page.getByTestId("voiceprint-adaptation-learned")).toContainText("0");
  });
}
