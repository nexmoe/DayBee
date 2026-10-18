import type { DesktopSnapshot } from "@daybee/shared";
import { expect, test } from "./fixtures";

async function bootstrap(page: import("@playwright/test").Page) {
  return page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
}

test("selects models in transcription settings and retains the choice after relaunch", async ({ launchApp }) => {
  let { app, page } = await launchApp();
  await page.setViewportSize({ width: 1000, height: 760 });
  await page.goto("/");
  await page.getByRole("tablist").getByRole("tab", { name: /Transcribe|转写/, exact: true }).click();
  const models = page.getByTestId("asr-model-settings");
  // The fixture writes an explicit Tiny selection, which must survive on Macs.
  expect((await bootstrap(page)).settings.recording.asrModel).toBe("whisper-tiny");
  await expect(models).toContainText("Whisper Tiny");
  await expect(models).toContainText("Whisper Base");
  await expect(models).toContainText("Whisper Small");
  await expect(models).toContainText("Qwen3 ASR 0.6B");
  await expect(models).toContainText("FireRedASR2 AED");
  await expect(models).toContainText("FireRedASR2 CTC");
  await expect(models).toContainText("SenseVoice Small");
  await page.getByTestId("asr-model-firered-asr2-ctc").getByRole("button").click();
  await expect.poll(async () => (await bootstrap(page)).settings.recording.asrModel).toBe("firered-asr2-ctc");
  await expect(page.getByTestId("asr-model-firered-asr2-ctc").getByRole("button")).toBeDisabled();
  await app.close();

  ({ app, page } = await launchApp());
  await expect.poll(async () => (await bootstrap(page)).settings.recording.asrModel).toBe("firered-asr2-ctc");
  await page.setViewportSize({ width: 1000, height: 760 });
  await page.goto("/");
  await page.getByRole("tablist").getByRole("tab", { name: /Transcribe|转写/, exact: true }).click();
  await page.screenshot({ path: "e2e/results/asr-models.png", animations: "disabled" });
  await page.evaluate(() => window.mygo!.call("DesktopService.StartRecording"));
  const qwen = page.getByTestId("asr-model-qwen3-0.6b").getByRole("button");
  await expect(qwen).toBeEnabled();
  await qwen.click();
  await expect.poll(async () => (await bootstrap(page)).settings.recording.asrModel).toBe("qwen3-0.6b");
  expect((await bootstrap(page)).status.recording).toBe(true);
  await expect(qwen).toBeDisabled();
  await page.evaluate(() => window.mygo!.call("DesktopService.StopRecording"));
  await app.close();
});

test("starts with the platform default and can select each Whisper model", async ({ launchApp }) => {
  const { app, page } = await launchApp({ platformASRDefault: true });
  await page.setViewportSize({ width: 1000, height: 760 });
  await page.goto("/");
  await page.getByRole("tablist").getByRole("tab", { name: /Transcribe|转写/, exact: true }).click();
  const snapshot = await bootstrap(page);
  const defaultModel = snapshot.asrModels.items.some(model => model.id === "speech-analyzer") ? "speech-analyzer" : "whisper-tiny";
  expect(snapshot.settings.recording.asrModel).toBe(defaultModel);
  expect(snapshot.asrModels.items[0].id).toBe(defaultModel);
  if (defaultModel === "speech-analyzer") {
    const system = page.getByTestId("asr-model-speech-analyzer");
    await expect(system).toContainText(/Managed by macOS|由 macOS 管理/);
    await expect(system).not.toContainText("INT8");
    await expect(system).not.toContainText("0 MB");
  }
  for (const id of ["whisper-base", "whisper-small", "whisper-tiny"]) {
    const row = page.getByTestId(`asr-model-${id}`);
    await row.getByRole("button").click();
    await expect.poll(async () => (await bootstrap(page)).settings.recording.asrModel).toBe(id);
    await expect(row.getByRole("button")).toBeDisabled();
  }
  await page.evaluate(() => window.mygo!.call("DesktopService.StartRecording"));
  expect((await bootstrap(page)).status.recording).toBe(true);
  expect((await bootstrap(page)).settings.recording.asrModel).toBe("whisper-tiny");
  await page.evaluate(() => window.mygo!.call("DesktopService.StopRecording"));
  await app.close();
});
