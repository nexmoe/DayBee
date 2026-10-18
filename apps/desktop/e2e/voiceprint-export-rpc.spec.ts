import { mkdirSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { DesktopSnapshot } from "@daybee/shared";
import { expect, test } from "./fixtures";

// Only the native folder picker is replaced. Export uses the packaged Go RPC
// against the isolated store's real history and files, without microphone use.
test("exports a completed WAV through the native service", async ({ launchApp, storeDir }) => {
  const recordings = join(storeDir, "recordings");
  const destination = join(storeDir, "samples");
  mkdirSync(recordings, { recursive: true });
  mkdirSync(destination);
  const basename = "daybee_20261012_120000_microphone.wav";
  const audioPath = join(recordings, basename);
  const wav = Buffer.alloc(44 + 16000 * 2);
  wav.write("RIFF");
  wav.writeUInt32LE(wav.length - 8, 4);
  wav.write("WAVEfmt ", 8);
  wav.writeUInt32LE(16, 16);
  wav.writeUInt16LE(1, 20);
  wav.writeUInt16LE(1, 22);
  wav.writeUInt32LE(16000, 24);
  wav.writeUInt32LE(32000, 28);
  wav.writeUInt16LE(2, 32);
  wav.writeUInt16LE(16, 34);
  wav.write("data", 36);
  wav.writeUInt32LE(wav.length - 44, 40);
  writeFileSync(audioPath, wav);
  const metadataPath = audioPath.replace(/\.wav$/, ".json");
  const metadata = JSON.stringify({
    audio_path: audioPath, audio_source: "microphone", status: "ok",
    segment_start_time: "2026-10-12T12:00:00+08:00",
    recording_session_id: "export-rpc-test", capture_device_id: "fixture-mic",
    echo_cancellation: false, text: "Private transcript must stay in history",
    transcript_chunks: [{ text: "Private transcript", voice_identity: "me" }]
  });
  writeFileSync(metadataPath, metadata);

  const { app, page } = await launchApp();
  await page.goto("/");
  await page.setViewportSize({ width: 1000, height: 900 });
  await expect.poll(async () => {
    const latest = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    return latest.history.some(item => item.id === audioPath);
  }).toBe(true);
  const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
  expect(snapshot.history.some(item => item.id === audioPath)).toBe(true);
  snapshot.settings.desktop.language = "en-US";
  await app.setFixture(snapshot);
  await page.getByRole("tablist").getByRole("tab", { name: "Transcribe", exact: true }).click();
  await page.getByTestId("voiceprint-diagnostics-toggle").click();
  await page.getByLabel("Finished recording").selectOption(audioPath);
  await page.evaluate((directory) => {
    const original = window.mygo!.call.bind(window.mygo);
    window.mygo!.call = ((method: string, ...args: unknown[]) => {
      if (method === "DesktopService.PickDirectory") return Promise.resolve(directory);
      return original(method, ...args);
    }) as NonNullable<Window["mygo"]>["call"];
  }, destination);
  await page.getByTestId("voiceprint-export-button").click();
  await expect(page.getByTestId("voiceprint-export-result")).toContainText(destination);

  const directories = readdirSync(destination);
  expect(directories).toHaveLength(1);
  const exported = join(destination, directories[0]!);
  expect(readdirSync(exported).sort()).toEqual([basename, "samples.json"].sort());
  expect(readFileSync(join(exported, basename))).toEqual(wav);
  const sidecarText = readFileSync(join(exported, "samples.json"), "utf8");
  const sidecar = JSON.parse(sidecarText);
  expect(sidecar.identityRanges).toEqual([]);
  expect(sidecar.files).toEqual([expect.objectContaining({
    file: basename, source: "microphone", sessionId: "export-rpc-test",
    device: "fixture-mic", echoCancellation: false
  })]);
  expect(sidecarText).not.toMatch(/transcript|embedding|voice_identity|Private/);
  expect(readFileSync(audioPath)).toEqual(wav);
  expect(readFileSync(metadataPath, "utf8")).toBe(metadata);
});
