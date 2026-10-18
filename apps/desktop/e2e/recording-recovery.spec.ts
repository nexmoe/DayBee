import { test, expect } from "@playwright/test";
import { DEFAULT_SETTINGS, DEFAULT_STATUS, type DesktopSnapshot } from "@daybee/shared";

for (const language of ["zh-CN", "en-US"] as const) {
  test(`interrupted recording shows recovery and can be stopped in ${language}`, async ({ page }) => {
    const snapshot: DesktopSnapshot = {
      asrModels: { items: [], busy: false, downloadingModel: null },
      app: { name: "DayBee", repositoryUrl: "https://github.com/nexmoe/DayBee", version: "0.8.6" },
      devices: [], engineReady: true, history: [],
      microphoneActivity: { supported: false, error: null, items: [] },
      voiceprint: { enrolled: false, phase: "idle", step: 0, speechSeconds: 0, targetSeconds: 5, level: 0, progress: null, message: "", error: null },
      permission: { state: "authorized", supported: true, message: "" },
      settings: { ...DEFAULT_SETTINGS, desktop: { ...DEFAULT_SETTINGS.desktop, language, voiceprintSetup: "skipped" } },
      status: { ...DEFAULT_STATUS, recording: false, recovering: true },
      updater: {
        currentVersion: "0.8.6", latestVersion: null, downloadedVersion: null,
        downloadedVersionReady: false, errorMessage: null, installDeferredUntilIdle: false,
        phase: "idle", statusMessage: ""
      },
      windowPinned: false
    };
    await page.addInitScript(snapshot => {
      let stops = 0;
      window.mygo = {
        platform: "darwin", windowId: 1, version: "0.3.4",
        call(method: string) {
          if (method === "DesktopService.Bootstrap") return Promise.resolve(structuredClone(snapshot));
          if (method === "DesktopService.StopRecording") {
            stops++;
            snapshot.status.recovering = false;
            return Promise.resolve(structuredClone(snapshot));
          }
          if (method === "DesktopService.StartRecording") throw new Error("Recovery control must stop, not start");
          if (method === "test.stops") return Promise.resolve(stops);
          return Promise.resolve();
        },
        on() { return () => {}; }
      } as unknown as NonNullable<Window["mygo"]>;
    }, snapshot);
    await page.setViewportSize({ width: 320, height: 620 });
    await page.goto("/");
    const recovering = language === "zh-CN" ? "正在恢复录音…" : "Reconnecting…";
    await expect(page.getByText(recovering, { exact: true })).toBeVisible();
    const stop = page.getByRole("button", { name: /Stop|停止/, exact: true });
    await expect(stop).toBeEnabled();
    await expect(stop).toBeInViewport();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: `/private/tmp/daybee-recovery-${language}.png`, animations: "disabled" });
    await stop.click();
    await expect(page.getByRole("button", { name: /Start recording|开始录音/, exact: true })).toBeVisible();
    await expect(page.getByText(recovering, { exact: true })).toHaveCount(0);
    await expect(page.getByText(language === "zh-CN" ? "空闲" : "Idle", { exact: true })).toBeVisible();
    expect(await page.evaluate(() => window.mygo!.call("test.stops"))).toBe(1);

    await page.goto("/");
    await page.setViewportSize({ width: 1000, height: 760 });
    await expect(page.getByText(recovering, { exact: true })).toBeVisible();
  });
}
