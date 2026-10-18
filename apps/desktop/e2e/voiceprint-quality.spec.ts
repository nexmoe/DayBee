import type { DesktopSnapshot } from "@daybee/shared";
import { expect, test } from "./fixtures";

for (const language of ["zh-CN", "en-US"] as const) {
  test(`explains room measurement, playback pause and unavailable reference in ${language}`, async ({ launchApp }) => {
    const { app, page } = await launchApp();
    await page.goto("/");
    const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    snapshot.settings.desktop.language = language;
    snapshot.voiceprint = {
      enrolled: true, phase: "recording", step: 0, targetSeconds: 8, speechSeconds: 0,
      level: 0, progress: null, error: null,
      message: "Stay quiet for a moment while the room is measured."
    };
    await app.setFixture(snapshot);
    const guide = page.getByTestId("voiceprint-guide");
    await expect(guide.getByRole("status")).toContainText(/环境|room/);
    await expect(guide.getByTestId("voiceprint-passage")).toHaveCount(0);

    snapshot.voiceprint.message = "Playback was detected. Enrollment is paused and this audio will not be saved.";
    await app.setFixture(snapshot);
    await expect(guide.getByRole("alert")).toContainText(/暂停|paused/);
    await expect(guide.getByTestId("voiceprint-passage")).toHaveCount(0);
    await expect(guide.getByRole("progressbar")).toHaveAttribute("aria-valuenow", "0");

    snapshot.voiceprint.message = "System playback cannot be checked. Pause videos yourself. Recording will continue.";
    await app.setFixture(snapshot);
    await expect(guide.getByTestId("voiceprint-quality")).toContainText(/自行|yourself/);
    await expect(guide.getByTestId("voiceprint-passage")).toBeVisible();

    for (const error of [
      "Background noise is too high. Move to a quieter place and record again.",
      "The microphone or echo cancellation changed. Record again with the same input.",
      "The enrollment does not separate from other speakers. Record your voice again in a quiet place."
    ]) {
      snapshot.voiceprint.phase = "error";
      snapshot.voiceprint.message = "";
      snapshot.voiceprint.error = error;
      await app.setFixture(snapshot);
      await expect(guide.getByRole("alert")).toBeVisible();
      await expect(guide.getByRole("alert")).toContainText(/之前保存|previously saved/);
      if (language === "zh-CN") await expect(guide).not.toContainText(error);
      await expect(guide.getByRole("button", { name: /重新开始|Start over/ })).toBeEnabled();
    }
    await app.close();
  });
}
