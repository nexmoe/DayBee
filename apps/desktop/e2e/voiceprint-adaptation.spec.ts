import type { AppSettings, DesktopSnapshot } from "@daybee/shared";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "./fixtures";

const model = "3dspeaker_speech_eres2netv2_sv_zh-cn_16k-common.onnx";

function basis(index: number): number[] {
  return Array.from({ length: 192 }, (_, slot) => (slot === index ? 1 : 0));
}

function writeEnrolledProfile(storeDir: string) {
  mkdirSync(join(storeDir, "voiceprint"), { recursive: true });
  const profile = {
    version: 2,
    model,
    embeddings: [basis(0)],
    createdAt: "2026-10-12T00:00:00Z",
    echoCancellation: false,
    adaptation: [basis(1), basis(2)],
    adaptationProgress: { learnedClips: 47, assistedClips: 8 }
  };
  const path = join(storeDir, "voiceprint", "profile.json");
  writeFileSync(path, JSON.stringify(profile));
  return path;
}

async function openTranscribe(page: import("@playwright/test").Page) {
  await page.goto("/");
  await page.setViewportSize({ width: 1000, height: 760 });
  await page.getByRole("tablist").getByRole("tab", { name: /Transcribe|转写/, exact: true }).click();
}

async function bootstrap(page: import("@playwright/test").Page) {
  return page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
}

async function saveLanguage(page: import("@playwright/test").Page, language: AppSettings["desktop"]["language"]) {
  const current = await bootstrap(page);
  await page.evaluate(async (settings) => {
    await window.mygo!.call("DesktopService.SaveSettings", settings);
  }, { ...current.settings, desktop: { ...current.settings.desktop, language } });
}

test("toggles adaptation with SaveSettings and clears learned samples on the real profile", async ({ launchApp, storeDir }) => {
  const profilePath = writeEnrolledProfile(storeDir);
  const { page } = await launchApp({ voiceprintSetup: "ready" });
  await openTranscribe(page);
  const field = page.getByTestId("voiceprint-adaptation-field");
  await saveLanguage(page, "zh-CN");
  const advanced = page.getByTestId("voiceprint-advanced");
  await expect(advanced).not.toHaveAttribute("open");
  await expect(field).not.toBeVisible();
  await expect(page.getByTestId("voiceprint-diagnostics")).not.toHaveAttribute("open");
  await expect(page.getByTestId("voiceprint-export-button")).not.toBeVisible();
  await expect(page.getByTestId("voiceprint-manage-field")).toContainText("转写时会将你的发言标记为「我」");
  await expect(page.getByTestId("voiceprint-manage-field")).not.toContainText(/0\/32|2\/32|学习我的声音/);
  await page.getByTestId("voiceprint-advanced-toggle").click();
  await expect(field).toBeVisible();
  await expect(field).toContainText("录音中持续适应");
  await expect(field.getByRole("switch")).not.toBeChecked();
  await expect(page.getByTestId("voiceprint-adaptation-count")).toHaveCount(0);
  await field.screenshot({ path: "e2e/results/voiceprint/adaptation-zh-CN.png", animations: "disabled" });
  await saveLanguage(page, "en-US");
  await expect(field).toContainText("Adapt during recording");
  await expect(field).toContainText("Your voiceprint works with this off");
  await expect(field.getByRole("switch")).not.toBeChecked();
  await field.screenshot({ path: "e2e/results/voiceprint/adaptation-en-US.png", animations: "disabled" });
  await field.getByRole("switch").click();
  await expect.poll(async () => (await bootstrap(page)).settings.recording.voiceprintAdaptation).toBe(true);
  const saved = JSON.parse(readFileSync(join(storeDir, "desktop-settings.json"), "utf8")) as { settings: AppSettings };
  expect(saved.settings.recording.voiceprintAdaptation).toBe(true);
  await expect(page.getByTestId("voiceprint-adaptation-learned")).toContainText("47");
  await expect(page.getByTestId("voiceprint-adaptation-assisted")).toContainText("8");
  await page.getByTestId("voiceprint-advanced-toggle").click();
  await expect(field).not.toBeVisible();
  await expect(page.getByTestId("voiceprint-advanced-toggle")).toContainText("Adaptation on");
  await page.getByTestId("voiceprint-advanced-toggle").click();
  await page.getByTestId("voiceprint-adaptation-clear").click();
  await expect.poll(async () => (await bootstrap(page)).voiceprint.adaptationCount ?? 0).toBe(0);
  await expect.poll(async () => (await bootstrap(page)).voiceprint.enrolled).toBe(true);
  await expect(page.getByTestId("voiceprint-adaptation-clear")).toBeDisabled();
  await expect(page.getByTestId("voiceprint-adaptation-learned")).toContainText("0");
  await expect(page.getByTestId("voiceprint-adaptation-assisted")).toContainText("0");
  const profile = JSON.parse(readFileSync(profilePath, "utf8")) as { embeddings: number[][]; adaptation?: number[][]; adaptationProgress?: { learnedClips: number; assistedClips: number } };
  expect(profile.embeddings[0][0]).toBe(1);
  expect(profile.embeddings[0]).toHaveLength(192);
  expect(profile.adaptation ?? []).toEqual([]);
  expect(profile.adaptationProgress).toEqual({ learnedClips: 0, assistedClips: 0 });
});

// Enrollment-in-progress is simulated with setFixture. The success path above uses the real manager.
test("disables adaptation controls while enrollment is in progress", async ({ launchApp }) => {
  const { app, page } = await launchApp({ voiceprintSetup: "ready" });
  await openTranscribe(page);
  const snapshot = await bootstrap(page);
  snapshot.voiceprint.enrolled = true;
  snapshot.voiceprint.phase = "recording";
  snapshot.voiceprint.adaptationCount = 2;
  await app.setFixture(snapshot);
  await page.getByTestId("voiceprint-advanced-toggle").click();
  const field = page.getByTestId("voiceprint-adaptation-field");
  await expect(field.getByRole("switch")).toBeDisabled();
  await expect(page.getByTestId("voiceprint-adaptation-clear")).toBeDisabled();
});
