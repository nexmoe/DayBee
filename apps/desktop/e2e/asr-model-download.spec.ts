import { test, expect, type Page } from "@playwright/test";
import { DEFAULT_SETTINGS, DEFAULT_STATUS, type ASRModel, type AppSettings, type DesktopSnapshot } from "@daybee/shared";

type ModelHarness = {
  snapshot: DesktopSnapshot;
  failSave: boolean;
  setRecording(recording: boolean): void;
  setBusy(busy: boolean): void;
  finishDownload(error?: string): void;
};
declare global { interface Window { modelHarness: ModelHarness } }

async function launchModels(page: Page, language: "zh-CN" | "en-US") {
  const snapshot: DesktopSnapshot = {
    asrModels: { busy: false, downloadingModel: null, items: [
      { id: "speech-analyzer", name: "SpeechAnalyzer", estimatedBytes: 0, ready: false },
      { id: "whisper-tiny", name: "Whisper Tiny", estimatedBytes: 116_204_861, ready: true },
      { id: "whisper-base", name: "Whisper Base", estimatedBytes: 207_557_382, ready: false },
      { id: "whisper-small", name: "Whisper Small", estimatedBytes: 639_387_718, ready: false },
      { id: "qwen3-0.6b", name: "Qwen3 ASR 0.6B", estimatedBytes: 878_702_423, ready: true },
      { id: "firered-asr2-aed", name: "FireRedASR2 AED", estimatedBytes: 838_589_068, ready: false },
      { id: "firered-asr2-ctc", name: "FireRedASR2 CTC", estimatedBytes: 520_516_278, ready: false },
      { id: "sensevoice-2025", name: "SenseVoice Small (2025)", estimatedBytes: 165_783_878, ready: false }
    ] },
    app: { name: "DayBee", repositoryUrl: "https://github.com/nexmoe/DayBee", version: "0.8.6" },
    devices: [], engineReady: true, history: [],
    microphoneActivity: { supported: false, error: null, items: [] },
    voiceprint: { enrolled: false, phase: "idle", step: 0, speechSeconds: 0, targetSeconds: 5, level: 0, progress: null, message: "", error: null },
    permission: { state: "authorized", supported: true, message: "" },
    // This fixture exercises an existing user's explicit Qwen selection.
    settings: { ...DEFAULT_SETTINGS, desktop: { ...DEFAULT_SETTINGS.desktop, language, voiceprintSetup: "skipped" },
      recording: { ...DEFAULT_SETTINGS.recording, asrModel: "qwen3-0.6b" } },
    status: { ...DEFAULT_STATUS, senseVoiceReady: true, vadReady: true },
    updater: { currentVersion: "0.8.6", latestVersion: null, downloadedVersion: null, downloadedVersionReady: false, errorMessage: null, installDeferredUntilIdle: false, phase: "idle", statusMessage: "" },
    windowPinned: false
  };
  await page.addInitScript(snapshot => {
    const listeners = new Set<(snapshot: DesktopSnapshot) => void>();
    let resolve: ((snapshot: DesktopSnapshot) => void) | undefined;
    let reject: ((error: Error) => void) | undefined;
    const emit = () => listeners.forEach(listener => listener(structuredClone(snapshot)));
    window.modelHarness = {
      snapshot, failSave: false,
      setRecording(recording) {
        snapshot.asrModels.busy = recording;
        snapshot.status.recording = recording;
        emit();
      },
      setBusy(busy) {
        snapshot.asrModels.busy = busy;
        emit();
      },
      finishDownload(error) {
        const model = snapshot.asrModels.items.find(item => item.id === snapshot.asrModels.downloadingModel)!;
        model.ready = !error;
        snapshot.asrModels.downloadingModel = null;
        snapshot.status.downloading = false;
        snapshot.status.downloadProgress = null;
        emit();
        if (error) reject?.(new Error(error));
        else resolve?.(structuredClone(snapshot));
      }
    };
    window.mygo = {
      platform: "darwin", windowId: 1, version: "0.3.4",
      call(method: string, ...args: unknown[]) {
        if (method === "DesktopService.Bootstrap") return Promise.resolve(structuredClone(snapshot));
        if (method === "DesktopService.PrepareASRModel") {
          const id = args[0] as ASRModel;
          if (snapshot.asrModels.items.find(item => item.id === id)?.ready) return Promise.resolve(structuredClone(snapshot));
          snapshot.asrModels.downloadingModel = id;
          snapshot.status.downloading = true;
          snapshot.status.downloadProgress = 42;
          emit();
          return new Promise<DesktopSnapshot>((yes, no) => { resolve = yes; reject = no; });
        }
        if (method === "DesktopService.SaveSettings") {
          if (window.modelHarness.failSave || (snapshot.asrModels.busy && !snapshot.status.recording)) return Promise.reject(new Error("Model selection failed"));
          snapshot.settings = args[0] as AppSettings;
          emit();
          return Promise.resolve(structuredClone(snapshot.settings));
        }
        return Promise.resolve();
      },
      on(name: string, listener: (snapshot: DesktopSnapshot) => void) {
        if (name === "desktop:snapshot") listeners.add(listener);
        return () => listeners.delete(listener);
      }
    } as unknown as NonNullable<Window["mygo"]>;
  }, snapshot);
  await page.setViewportSize({ width: 1000, height: 900 });
  await page.goto("/");
  await page.getByRole("tablist").getByRole("tab", { name: /Transcribe|转写/, exact: true }).click();
}

test("downloads and switches models without stopping recording", async ({ page }) => {
  await launchModels(page, "zh-CN");
  await page.evaluate(() => window.modelHarness.setRecording(true));
  const row = page.getByTestId("asr-model-firered-asr2-ctc");
  await expect(row.getByRole("button")).toBeEnabled();
  await expect(row.getByRole("button")).toHaveAccessibleName("下载并使用 FireRedASR2 CTC");
  await row.getByRole("button").click();
  await expect(row.getByRole("progressbar")).toHaveAttribute("value", "42");
  await page.evaluate(() => window.modelHarness.finishDownload());
  await expect(row).toContainText("已下载");
  await expect(row.getByRole("button")).toHaveAccessibleName("当前模型");
  expect(await page.evaluate(() => window.modelHarness.snapshot.settings.recording.asrModel)).toBe("firered-asr2-ctc");
  expect(await page.evaluate(() => window.modelHarness.snapshot.status.recording)).toBe(true);
  const previous = page.getByTestId("asr-model-qwen3-0.6b");
  await expect(previous.getByRole("button")).toBeEnabled();
  await previous.getByRole("button").click();
  await expect(previous.getByRole("button")).toHaveAccessibleName("当前模型");
  expect(await page.evaluate(() => window.modelHarness.snapshot.status.recording)).toBe(true);
});

test("prepares the macOS system model without displaying download size or INT8 inference", async ({ page }) => {
  await launchModels(page, "en-US");
  const row = page.getByTestId("asr-model-speech-analyzer");
  await expect(row).toContainText("Managed by macOS");
  await expect(row).not.toContainText("0 MB");
  await expect(row).not.toContainText("INT8");
  await expect(row.getByRole("button")).toHaveAccessibleName("Prepare and use SpeechAnalyzer");
  await row.getByRole("button").click();
  await expect(row).toContainText("Preparing…");
  await page.evaluate(() => window.modelHarness.finishDownload());
  await expect(row.getByRole("button")).toHaveAccessibleName("Current model");
  await expect(row).toContainText("Ready");
  expect(await page.evaluate(() => window.modelHarness.snapshot.settings.recording.asrModel)).toBe("speech-analyzer");
  await expect(page.getByText("SpeechAnalyzer uses one language at a time.", { exact: false })).toBeVisible();
});

test("recording started during a download can activate the downloaded model", async ({ page }) => {
  await launchModels(page, "en-US");
  const row = page.getByTestId("asr-model-sensevoice-2025");
  await row.getByRole("button").click();
  await page.evaluate(() => {
    window.modelHarness.setRecording(true);
    window.modelHarness.finishDownload();
  });
  await expect(row).toContainText("Downloaded");
  await expect(row.getByRole("button")).toHaveAccessibleName("Current model");
  expect(await page.evaluate(() => window.modelHarness.snapshot.settings.recording.asrModel)).toBe("sensevoice-2025");
  expect(await page.evaluate(() => window.modelHarness.snapshot.status.recording)).toBe(true);
});

test("keeps activation blocked for busy work outside live recording", async ({ page }) => {
  await launchModels(page, "zh-CN");
  await page.evaluate(() => window.modelHarness.setBusy(true));
  const row = page.getByTestId("asr-model-firered-asr2-ctc");
  await expect(row.getByRole("button")).toHaveAccessibleName("下载 FireRedASR2 CTC");
  await row.getByRole("button").click();
  await page.evaluate(() => window.modelHarness.finishDownload());
  await expect(row).toContainText("已下载");
  await expect(row.getByRole("button")).toBeDisabled();
  expect(await page.evaluate(() => window.modelHarness.snapshot.settings.recording.asrModel)).toBe("qwen3-0.6b");
  await page.evaluate(() => window.modelHarness.setBusy(false));
  await row.getByRole("button").click();
  await expect(row.getByRole("button")).toHaveAccessibleName("当前模型");
});

for (const language of ["zh-CN", "en-US"] as const) {
  test(`shows download progress and activates only after success in ${language}`, async ({ page }) => {
    await launchModels(page, language);
    const row = page.getByTestId("asr-model-firered-asr2-aed");
    await row.getByRole("button").click();
    await expect(row.getByRole("progressbar")).toHaveAttribute("value", "42");
    await expect(page.getByTestId("asr-model-qwen3-0.6b").getByRole("button")).toBeDisabled();
    expect(await page.evaluate(() => window.modelHarness.snapshot.settings.recording.asrModel)).toBe("qwen3-0.6b");
    await page.evaluate(() => window.modelHarness.finishDownload());
    await expect(row.getByRole("button")).toHaveAccessibleName(/Current model|当前模型/);
    await expect(row.getByRole("progressbar")).toHaveCount(0);
    expect(await page.evaluate(() => window.modelHarness.snapshot.settings.recording.asrModel)).toBe("firered-asr2-aed");
    await page.screenshot({ path: `e2e/results/asr-models-${language}.png`, animations: "disabled" });
  });
}

test("failed downloads and rejected saves retain the original model and allow retry", async ({ page }) => {
  await launchModels(page, "en-US");
  const row = page.getByTestId("asr-model-firered-asr2-ctc");
  await row.getByRole("button").click();
  await page.evaluate(() => window.modelHarness.finishDownload("Download interrupted"));
  await expect(page.getByText("Download interrupted", { exact: true })).toBeVisible();
  await expect(row.getByRole("button")).toBeEnabled();
  expect(await page.evaluate(() => window.modelHarness.snapshot.settings.recording.asrModel)).toBe("qwen3-0.6b");
  await row.getByRole("button").click();
  await page.evaluate(() => { window.modelHarness.failSave = true; window.modelHarness.finishDownload(); });
  await expect(page.getByText("Model selection failed", { exact: true })).toBeVisible();
  await expect(page.getByTestId("asr-model-qwen3-0.6b").getByRole("button")).toHaveAccessibleName("Current model");
  await expect(row.getByRole("button")).toBeEnabled();
  await page.evaluate(() => { window.modelHarness.failSave = false; });
  await row.getByRole("button").click();
  await expect(row.getByRole("button")).toHaveAccessibleName("Current model");
  await page.setViewportSize({ width: 720, height: 760 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});
