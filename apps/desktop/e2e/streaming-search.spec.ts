import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import type { DesktopSnapshot } from "@daybee/shared";
import { expect, test } from "./fixtures";

async function startRecording(page: Page) {
  await page.getByRole("button", { name: /Start recording|开始录音/ }).click();
  await expect.poll(async () => (await page.evaluate(() =>
    window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"))).status
  ).toMatchObject({ recording: true, asrEnabled: true });
}

for (const role of ["compact", "main"] as const) {
  test(`streams completed speech into the ${role} timeline before recording stops`, async ({ launchApp }) => {
    const { app, page } = await launchApp();
    await startRecording(page);
    if (role === "main") {
      await page.getByRole("button", { name: /Open main window|打开主窗口/ }).click();
      await page.goto("/?window=main");
      await page.setViewportSize({ width: 1000, height: 760 });
    }
    const timeline = page.getByRole("region", { name: role === "main" ? /Home|首页/ : /Transcription timeline|转写时间线/, exact: true });
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

test("searches all saved speech, highlights literal matches and updates active searches", async ({ launchApp }, testInfo) => {
  const { app, page } = await launchApp();
  const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
  const directory = snapshot.settings.recording.outputDir;
  mkdirSync(directory, { recursive: true });
  const texts = ["录音", "质量与 PROJECT [v2] .* 的讨论。\n\n其余结论。", "最新记录。\n\nPROJECT 的最新结论。"];
  const previousDay = Date.now() - 86_400_000;
  texts.forEach((text, index) => writeFileSync(join(directory, `${index}.json`), JSON.stringify({
    audio_path: join(directory, `${index}.flac`), segment_start_time: new Date(previousDay + index * 30_000).toISOString(), status: "ok", text
  })));
  await app.refreshHistory();
  await startRecording(page);
  await page.getByRole("button", { name: /Open main window|打开主窗口/ }).click();
  await page.goto("/?window=main");
  await page.setViewportSize({ width: 1000, height: 760 });
  const home = page.getByRole("region", { name: /Home|首页/, exact: true });
  const search = page.getByRole("searchbox", { name: /Search transcripts|搜索转写/ });
  await expect(home.getByRole("article")).toHaveCount(5);
  await search.fill("质量与");
  await expect(home.getByRole("article")).toHaveCount(1);
  await expect(home.locator("mark")).toHaveText("质量与");
  await expect(home).toContainText("PROJECT [v2] .* 的讨论。");
  await search.fill("[v2] .*");
  await expect(home.locator("mark")).toHaveText("[v2] .*");
  await search.fill("不存在的关键词");
  await expect(home.getByRole("heading", { name: /No matching activity|没有匹配的活动/ })).toBeVisible();
  await expect(home.getByRole("article")).toHaveCount(0);
  await search.press("Escape");
  await expect(search).toHaveValue("");
  await expect(home.getByRole("article")).toHaveCount(5);
  await search.fill("project");
  await expect(home.getByRole("article")).toHaveCount(2);
  await expect(home.locator("mark")).toHaveText(["PROJECT", "PROJECT"]);
  await expect(home.getByRole("article").first()).toContainText("最新结论");
  await app.completeSpeech("其他实时内容。\n\nNew PROJECT result.");
  await expect(home).toContainText("New PROJECT result.", { timeout: 1000 });
  await expect(home.getByRole("article")).toHaveCount(3);
  await expect(search).toHaveValue("project");
  await page.getByRole("navigation").getByRole("button", { name: /General|通用/, exact: true }).click();
  await expect(search).toHaveCount(0);
  await page.getByRole("navigation").getByRole("button", { name: /Home|首页/, exact: true }).click();
  await expect(search).toHaveValue("project");
  await page.setViewportSize({ width: 720, height: 540 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.addStyleTag({ content: "html { background: #f5f5f7; }" });
  await page.evaluate(() => document.fonts.ready);
  const screenshot = testInfo.outputPath("search-results.png");
  await page.screenshot({ path: screenshot, animations: "disabled" });
  await testInfo.attach("search-results", { path: screenshot, contentType: "image/png" });
  await page.getByRole("button", { name: /Clear search|清除搜索/ }).click();
  await expect(search).toHaveValue("");
  await expect(home.getByRole("article")).toHaveCount(7);
  await app.close();
});
