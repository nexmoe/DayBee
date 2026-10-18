import type { DesktopSnapshot } from "@daybee/shared";
import { expect, test } from "./fixtures";

for (const role of ["compact"] as const) {
  test(`streams completed speech into the ${role} timeline before recording stops`, async ({ launchApp }) => {
    const { app, page } = await launchApp();
    await page.getByRole("button", { name: /Start recording|开始录音/ }).click();
    await expect.poll(async () => {
      const { status } = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
      return status.recording && status.asrEnabled;
    }).toBe(true);
    if (role === "compact") {
      await page.getByRole("tab", { name: /Home|首页/, exact: true }).click();
      await page.goto("/");
      await page.setViewportSize({ width: 1000, height: 760 });
    }
    const timeline = page.getByRole("region", { name: role === "compact" ? /Home|首页/ : /Transcription timeline|转写时间线/, exact: true });
    await expect(timeline.getByRole("article")).toHaveCount(0);
    await app.completeSpeech("第一段已经识别完成。");
    await expect(timeline.locator(".transcript-text")).toHaveText("第一段已经识别完成。", { timeout: 1000 });
    await app.completeSpeech("第二段继续实时出现。");
    await expect(timeline.locator(".transcript-text"))
      .toHaveText(["第一段已经识别完成。", "第二段继续实时出现。"], { timeout: 1000 });
    const recording = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    expect(recording.status.recording).toBe(true);
    await page.evaluate(() => window.mygo!.call("DesktopService.StopRecording"));
    await expect(timeline.locator(".transcript-text"))
      .toHaveText(["第一段已经识别完成。", "第二段继续实时出现。"]);
    if (role === "compact") await expect(page.getByText(/Live transcript|实时转写/, { exact: true })).toHaveCount(0);
    await app.close();
  });
}
