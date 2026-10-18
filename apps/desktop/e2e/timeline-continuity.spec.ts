import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { DesktopSnapshot } from "@daybee/shared";
import { expect, test } from "./fixtures";

for (const role of ["compact", "main"] as const) {
  test(`separates distant recordings and silence inside one file in the ${role} timeline`, async ({ launchApp }, testInfo) => {
    const { app, page } = await launchApp();
    const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    const directory = snapshot.settings.recording.outputDir;
    mkdirSync(directory, { recursive: true });
    const records = [
      { name: "first", at: "2026-10-09T11:14:33+08:00", text: "然后。谁？你是谁？" },
      { name: "second", at: "2026-10-09T12:18:57+08:00", text: "是是是。真的好。" },
      { name: "third", at: "2026-10-09T12:56:20+08:00", text: "测试测试。\n行行行。" }
    ];
    for (const record of records) writeFileSync(join(directory, `${record.name}.json`), JSON.stringify({
      audio_path: join(directory, `${record.name}.flac`), segment_start_time: record.at, status: "ok", text: record.text
    }));
    await app.refreshHistory();
    if (role === "main") {
      await page.goto("/?window=main");
      await page.setViewportSize({ width: 1000, height: 760 });
    }
    const timeline = page.getByRole("region", { name: role === "main" ? /Home|首页/ : /Transcription timeline|转写时间线/, exact: true });
    const text = timeline.locator(".transcript-text");
    const sentences = ["测试测试。", "行行行。", "是是是。", "真的好。", "然后。", "谁？", "你是谁？"];
    await expect(text).toHaveText(sentences);
    expect(await timeline.locator("time").evaluateAll(elements => elements.map(element => element.getAttribute("datetime"))))
      .toEqual([...records].reverse().map(record => new Date(record.at).toISOString()));
    await page.addStyleTag({ content: "html { background: #f5f5f7; }" });
    await page.evaluate(() => document.fonts.ready);
    const screenshot = testInfo.outputPath(`timeline-separated-${role}.png`);
    await page.screenshot({ path: screenshot, animations: "disabled" });
    await testInfo.attach(`timeline-separated-${role}`, { path: screenshot, contentType: "image/png" });
    if (role === "main") {
      const search = page.getByRole("searchbox", { name: /Search transcripts|搜索转写/ });
      await search.fill("真的好。测试测试");
      await expect(text).toHaveCount(0);
      await search.press("Escape");
      await expect(text).toHaveCount(7);
    }
    // One long archive can contain two short utterances separated by silence.
    writeFileSync(join(directory, "long.json"), JSON.stringify({
      audio_path: join(directory, "long.flac"), segment_start_time: "2026-10-09T13:00:00+08:00",
      status: "ok", text: "长录音前面的语音。\n停顿后刚刚完成的语音。", recording_session_id: "one-long-session",
      transcript_chunks: [
        { text: "长录音前面的语音。", startedAt: "2026-10-09T13:01:00+08:00", endedAt: "2026-10-09T13:01:05+08:00" },
        { text: "停顿后刚刚完成的语音。", startedAt: "2026-10-09T14:40:00+08:00", endedAt: "2026-10-09T14:40:04+08:00" }
      ]
    }));
    await app.refreshHistory();
    await expect(text).toHaveText(["停顿后刚刚完成的语音。", "长录音前面的语音。", ...sentences]);
    const ranges = await timeline.locator(".transcript-entry-header").evaluateAll(headers => headers.map(header =>
      Array.from(header.querySelectorAll("time")).map(time => time.dateTime)));
    expect(ranges.slice(0, 2)).toEqual([
      ["2026-10-09T06:40:00.000Z", "2026-10-09T06:40:04.000Z"],
      ["2026-10-09T05:01:00.000Z", "2026-10-09T05:01:05.000Z"]
    ]);
    expect(ranges.every(range => range.length === 1 || Date.parse(range[1]) - Date.parse(range[0]) <= 300_000)).toBe(true);
    await app.close();
  });
}
