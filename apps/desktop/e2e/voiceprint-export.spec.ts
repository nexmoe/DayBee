import type { DesktopSnapshot, RecordingHistoryItem } from "@daybee/shared";
import { expect, test } from "./fixtures";

const finishedClip: RecordingHistoryItem = {
  audioSource: "microphone",
  audioPath: "/recordings/finished.wav",
  folderPath: "/recordings",
  id: "clip-finished",
  inputDevice: null,
  startedAt: "2026-10-09T14:00:00Z",
  status: "ok",
  textPreview: "Finished meeting",
  sessionId: "session-finished",
  transcriptChunks: []
};

const unfinishedClip: RecordingHistoryItem = {
  ...finishedClip,
  audioPath: "/recordings/open.wav",
  id: "clip-open",
  status: "recording",
  textPreview: "Still recording"
};

async function openTranscribe(page: import("@playwright/test").Page) {
  await page.goto("/");
  await page.setViewportSize({ width: 1000, height: 800 });
  await page.getByRole("tablist").getByRole("tab", { name: /Transcribe|转写/, exact: true }).click();
}

async function bootstrap(page: import("@playwright/test").Page) {
  return page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
}

async function stubExport(page: import("@playwright/test").Page, mode: "cancel" | "export") {
  await page.evaluate((nextMode) => {
    const host = window as unknown as {
      __daybeeCalls?: string[][];
      __daybeeMode?: string;
      __daybeeReady?: boolean;
    };
    host.__daybeeCalls = [];
    host.__daybeeMode = nextMode;
    if (host.__daybeeReady) return;
    host.__daybeeReady = true;
    const original = window.mygo!.call.bind(window.mygo);
    window.mygo!.call = ((method: string, ...args: unknown[]) => {
      host.__daybeeCalls?.push([method, ...args.map(String)]);
      if (method === "DesktopService.PickDirectory") {
        return Promise.resolve(host.__daybeeMode === "cancel" ? null : "/tmp/daybee-voiceprint-dest");
      }
      if (method === "DesktopService.ExportVoiceprintSamples") {
        return Promise.resolve("/tmp/daybee-voiceprint-dest/voiceprint-sample-1");
      }
      if (method === "DesktopService.OpenRecordingFolder") return Promise.resolve();
      return original(method, ...args);
    }) as NonNullable<Window["mygo"]>["call"];
  }, mode);
}

for (const language of ["zh-CN", "en-US"] as const) {
  test(`shows playback reference and device mismatch hints in ${language}`, async ({ launchApp }) => {
    const { app, page } = await launchApp();
    await openTranscribe(page);
    const snapshot = await bootstrap(page);
    snapshot.settings.desktop.language = language;
    snapshot.settings.recording.disableAsr = false;
    snapshot.status.asrEnabled = true;
    snapshot.status.recording = true;
    snapshot.voiceprint.enrolled = true;
    snapshot.status.voiceprintReference = "unavailable";
    snapshot.status.voiceprintContextMismatch = false;
    await app.setFixture(snapshot);
    const echoed = await bootstrap(page);
    expect(echoed.status.voiceprintReference).toBe("unavailable");
    const hints = page.getByTestId("voiceprint-runtime-hints");
    await expect(hints.getByRole("status")).toContainText(language === "zh-CN" ? "普通录音会继续" : "Recording continues");
    await expect(hints.getByRole("alert")).toHaveCount(0);

    snapshot.status.voiceprintReference = "available";
    snapshot.status.voiceprintContextMismatch = true;
    await app.setFixture(snapshot);
    // The settings card shows reference failures, while an available reference
    // needs no runtime notice. A device mismatch remains actionable.
    await expect(hints.getByRole("status")).toHaveCount(0);
    await expect(hints.getByRole("alert")).toContainText(language === "zh-CN" ? "需要重新录入" : "Record it again");
    await expect(hints).not.toContainText(/cohort|z-score|embedding|向量/);

    snapshot.status.recording = false;
    await app.setFixture(snapshot);
    await expect(hints.getByRole("status")).toHaveCount(0);
    await expect(hints.getByRole("alert")).toBeVisible();

    snapshot.settings.recording.disableAsr = true;
    await app.setFixture(snapshot);
    await expect(hints).toHaveCount(0);
  });

  test(`exports one finished recording after a folder is chosen in ${language}`, async ({ launchApp }) => {
    const { app, page } = await launchApp();
    await openTranscribe(page);
    const snapshot = await bootstrap(page);
    snapshot.settings.desktop.language = language;
    snapshot.history = [];
    await app.setFixture(snapshot);
    await expect(page.getByTestId("voiceprint-diagnostics")).not.toHaveAttribute("open");
    await expect(page.getByTestId("voiceprint-export-button")).not.toBeVisible();
    await page.getByTestId("voiceprint-diagnostics-toggle").click();
    const button = page.getByTestId("voiceprint-export-button");
    await expect(button).toBeDisabled();
    await expect(page.getByTestId("voiceprint-export-field")).toContainText(language === "zh-CN" ? "没有可导出" : "No finished recording");

    snapshot.history = [unfinishedClip, finishedClip];
    await app.setFixture(snapshot);
    await expect(button).toBeEnabled();
    await expect(page.getByLabel(language === "zh-CN" ? "已完成的录音" : "Finished recording").locator("option")).toHaveCount(1);

    await stubExport(page, "cancel");
    await button.click();
    await expect.poll(() => page.evaluate(() => (window as unknown as { __daybeeCalls?: string[][] }).__daybeeCalls?.map(call => call[0]) ?? [])).toContain("DesktopService.PickDirectory");
    const cancelled = await page.evaluate(() => (window as unknown as { __daybeeCalls?: string[][] }).__daybeeCalls ?? []);
    expect(cancelled.map(call => call[0])).not.toContain("DesktopService.ExportVoiceprintSamples");
    await expect(page.getByTestId("voiceprint-export-result")).toHaveCount(0);

    await stubExport(page, "export");
    await button.click();
    await expect(page.getByTestId("voiceprint-export-result")).toContainText("/tmp/daybee-voiceprint-dest/voiceprint-sample-1");
    await page.getByRole("button", { name: language === "zh-CN" ? "打开所在文件夹" : "Show in folder" }).click();
    const calls = await page.evaluate(() => (window as unknown as { __daybeeCalls?: string[][] }).__daybeeCalls ?? []);
    expect(calls).toContainEqual(["DesktopService.ExportVoiceprintSamples", "clip-finished", "/tmp/daybee-voiceprint-dest"]);
    expect(calls).toContainEqual(["DesktopService.OpenRecordingFolder", "/tmp/daybee-voiceprint-dest/voiceprint-sample-1"]);
  });
}
