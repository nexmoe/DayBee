import { test, expect, type Page } from "@playwright/test";
import { DEFAULT_SETTINGS, DEFAULT_STATUS, type AutoUpdatePhase, type DesktopSnapshot } from "@daybee/shared";

type UpdateHarness = {
  calls: number;
  snapshot: DesktopSnapshot;
  finish(error?: string): void;
  emit(phase: AutoUpdatePhase): void;
};

declare global {
  interface Window { updateHarness: UpdateHarness }
}

async function launchUpdater(page: Page, language: "zh-CN" | "en-US" = "zh-CN") {
  const snapshot: DesktopSnapshot = {
    asrModels: { items: [], busy: false, downloadingModel: null },
    app: { name: "DayBee", repositoryUrl: "https://github.com/nexmoe/DayBee", version: "0.8.6" },
    devices: [], engineReady: true, history: [],
    microphoneActivity: { supported: true, error: null, items: [] },
    voiceprint: { enrolled: false, phase: "idle", step: 0, speechSeconds: 0, targetSeconds: 5, level: 0, progress: null, message: "", error: null },
    permission: { state: "authorized", supported: true, message: "" },
    settings: { ...DEFAULT_SETTINGS, desktop: { ...DEFAULT_SETTINGS.desktop, language, voiceprintSetup: "skipped" } },
    status: { ...DEFAULT_STATUS, recording: true, elapsed: "12:34:56" },
    updater: {
      currentVersion: "0.8.6", latestVersion: "0.8.7", downloadedVersion: null,
      downloadedVersionReady: false, errorMessage: null, installDeferredUntilIdle: false,
      phase: "idle", statusMessage: ""
    },
    windowPinned: false
  };
  await page.addInitScript(snapshot => {
    const listeners = new Set<(snapshot: DesktopSnapshot) => void>();
    let resolveInstall: (() => void) | undefined;
    let rejectInstall: ((error: Error) => void) | undefined;
    const harness: UpdateHarness = {
      calls: 0, snapshot,
      emit(phase) {
        snapshot.updater.phase = phase;
        for (const listener of listeners) listener(structuredClone(snapshot));
      },
      finish(error) {
        if (error) {
          snapshot.updater.errorMessage = error;
          this.emit("error");
          rejectInstall?.(new Error(error));
        } else {
          snapshot.updater.downloadedVersion = snapshot.updater.latestVersion;
          snapshot.updater.downloadedVersionReady = true;
          snapshot.status.recording = false;
          this.emit("downloaded");
          resolveInstall?.();
        }
      }
    };
    window.updateHarness = harness;
    window.mygo = {
      platform: "darwin", windowId: 1, version: "0.3.4",
      call(method: string) {
        if (method === "DesktopService.Bootstrap") return Promise.resolve(structuredClone(snapshot));
        if (method === "DesktopService.InstallUpdate") {
          harness.calls++;
          return new Promise<void>((resolve, reject) => {
            resolveInstall = resolve;
            rejectInstall = reject;
          });
        }
        return Promise.resolve();
      },
      on(name: string, listener: (snapshot: DesktopSnapshot) => void) {
        if (name === "desktop:snapshot") listeners.add(listener);
        return () => { listeners.delete(listener); };
      }
    } as NonNullable<Window["mygo"]>;
  }, snapshot);
  await page.goto("/");
  await expect(page.getByRole("button", { name: /Stop|停止/, exact: true })).toBeVisible();
}

async function emit(page: Page, phase: AutoUpdatePhase) {
  await page.evaluate(phase => window.updateHarness.emit(phase), phase);
}

test("new update appears left of Pin, starts immediately and prevents duplicate clicks", async ({ page }) => {
  await page.setViewportSize({ width: 420, height: 620 });
  await launchUpdater(page);
  const update = page.getByRole("button", { name: "有新更新", exact: true });
  await expect(update).toHaveCount(0);
  await emit(page, "available");
  await expect(update).toBeVisible();
  await expect(update).toHaveText("");
  const pin = page.getByRole("button", { name: "定住窗口", exact: true });
  await expect(pin).toBeVisible();
  const [updateBounds, pinBounds] = await Promise.all([update.boundingBox(), pin.boundingBox()]);
  expect(updateBounds!.x + updateBounds!.width).toBeLessThan(pinBounds!.x);
  expect(updateBounds!.y).toBe(pinBounds!.y);
  await expect(update).toHaveAttribute("title", /0.8.7.*重启.*录音.*保存/);

  await update.click();
  const updating = page.getByRole("button", { name: "更新中…", exact: true });
  await expect(updating).toBeDisabled();
  await expect(updating).toHaveAttribute("aria-busy", "true");
  await updating.dispatchEvent("click");
  expect(await page.evaluate(() => window.updateHarness.calls)).toBe(1);
  await page.evaluate(() => window.updateHarness.finish());
  await expect(page.getByRole("button", { name: "正在重启…" })).toBeDisabled();
});

test("failed update shows the error and can be retried", async ({ page }) => {
  await launchUpdater(page);
  await emit(page, "available");
  await page.getByRole("button", { name: "有新更新", exact: true }).click();
  await page.evaluate(() => window.updateHarness.finish("signature rejected"));
  await expect(page.getByText("安装更新失败", { exact: true })).toBeVisible();
  await expect(page.getByText("signature rejected", { exact: true })).toBeVisible();
  const update = page.getByRole("button", { name: "有新更新", exact: true });
  await expect(update).toBeEnabled();
  await update.click();
  expect(await page.evaluate(() => window.updateHarness.calls)).toBe(2);
  await page.evaluate(() => window.updateHarness.finish());
  await expect(page.getByRole("button", { name: "正在重启…" })).toBeDisabled();
});

for (const language of ["zh-CN", "en-US"] as const) {
  test(`update controls fit narrow windows in ${language}`, async ({ page }) => {
    await launchUpdater(page, language);
    for (const width of [320, 380, 420]) {
      await page.setViewportSize({ width, height: 620 });
      await emit(page, "available");
      await expect(page.getByRole("button", { name: /Update available|有新更新/, exact: true })).toBeVisible();
      await expect(page.getByRole("button", { name: /Stop|停止/, exact: true })).toBeInViewport();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      const bounds = await page.getByRole("button", { name: /Stop|停止/, exact: true }).boundingBox();
      expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width);
    }
    for (const colorScheme of ["light", "dark"] as const) {
      await page.emulateMedia({ colorScheme });
      await page.evaluate(colorScheme => {
        document.body.style.backgroundColor = colorScheme === "dark" ? "#242424" : "#ededed";
      }, colorScheme);
      if (colorScheme === "dark") await expect(page.locator("html")).toHaveClass(/dark/);
      await page.screenshot({ path: `/private/tmp/daybee-updater-icon-${language}-${colorScheme}.png` });
    }
    for (const phase of ["checking", "idle", "unavailable"] as const) {
      await emit(page, phase);
      await expect(page.getByRole("button", { name: /Update available|有新更新/, exact: true })).toHaveCount(0);
    }
  });
}
