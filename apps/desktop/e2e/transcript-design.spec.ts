import type { DesktopSnapshot, RecordingHistoryItem } from "@daybee/shared";
import { expect, test } from "./fixtures";

const at = (seconds: number) => new Date(Date.parse("2026-10-09T10:00:00Z") + seconds * 1000).toISOString();
const icon = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aDLsAAAAASUVORK5CYII=";
const record = (id: string, audioSource: "microphone" | "system", transcriptChunks: RecordingHistoryItem["transcriptChunks"]): RecordingHistoryItem => ({
  id, audioSource, sessionId: id, audioPath: `/recordings/${id}.flac`, folderPath: "/recordings", inputDevice: null,
  status: "ok", startedAt: transcriptChunks[0].startedAt, textPreview: transcriptChunks.map(chunk => chunk.text).join("\n"), transcriptChunks
});

for (const role of ["compact", "main"] as const) for (const language of ["zh-CN", "en-US"] as const) for (const theme of ["light", "dark"] as const) {
  test(`renders connected speaker nodes in ${role} ${language} ${theme}`, async ({ launchApp }, testInfo) => {
    const { app, page } = await launchApp();
    const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    snapshot.settings.desktop = { ...snapshot.settings.desktop, language, theme };
    snapshot.history = [
      record("meeting", "microphone", [
        { text: "今天讨论录音质量。需要完整保留每个人的发言。", voiceIdentity: "me", startedAt: at(0), endedAt: at(6) },
        { text: "这段声音还没有匹配到声纹。内容仍然可以正常阅读。", startedAt: at(7), endedAt: at(12) },
        { text: "We agreed on the next step. Keep sentences flowing as one paragraph.", voiceIdentity: "me", startedAt: at(13), endedAt: at(18) }
      ]),
      record("playback", "system", [{ text: "视频中播放的声音，与麦克风分开标识。", voiceIdentity: "me", startedAt: at(-30), endedAt: at(-25) }]),
      { ...record("legacy", "microphone", [{ text: "旧录音没有声纹和有效时间，也应当可以阅读。", startedAt: "invalid", endedAt: "invalid" }]), startedAt: "invalid" }
    ];
    snapshot.microphoneActivity = { supported: true, error: null, items: [
      { id: "meeting-app", appId: "wetype", appName: "微信输入法", appIcon: icon, startedAt: at(-1), endedAt: at(20) },
      { id: "other-app", appId: "long-app", appName: "会议应用LongApplicationNameWithoutSpaces用于验证窗口内换行", appIcon: "data:image/png;base64,broken", startedAt: at(-1), endedAt: at(20) }
    ] };
    await app.setFixture(snapshot);
    if (role === "main") await page.goto("/?window=main");
    await page.setViewportSize({ width: role === "main" ? 1000 : 420, height: 900 });
    await page.emulateMedia({ colorScheme: theme });
    await expect(page.locator("html")).toHaveClass(theme === "dark" ? /dark/ : /^$/);
    const timeline = page.getByRole("list", { name: /Transcription timeline|转写时间线/ });
    const meeting = timeline.getByRole("article").filter({ has: page.locator(".recording-context") });
    const me = language === "zh-CN" ? "我" : "Me";
    const unknown = language === "zh-CN" ? "身份未确认" : "Identity unconfirmed";
    await expect(meeting.locator(".transcript-speaker-name")).toHaveText([me, unknown, me]);
    await expect(meeting.locator(".transcript-prose")).toHaveText([
      "We agreed on the next step. Keep sentences flowing as one paragraph.",
      "这段声音还没有匹配到声纹。内容仍然可以正常阅读。",
      "今天讨论录音质量。需要完整保留每个人的发言。"
    ]);
    expect(await meeting.locator(".transcript-time-range").evaluateAll(ranges => ranges.map(range =>
      Array.from(range.querySelectorAll("time"), time => time.dateTime))))
      .toEqual([[at(13), at(18)], [at(7), at(12)], [at(0), at(6)]]);
    const microphone = language === "zh-CN" ? "麦克风" : "Microphone";
    await expect(meeting.getByTestId("audio-source-label")).toHaveText([microphone, microphone, microphone]);
    expect(await timeline.getByRole("article").evaluateAll(articles => articles.every(article =>
      article.matches(".transcript-turn") && article.querySelectorAll('[data-testid="audio-source-label"]').length === 1))).toBe(true);
    await expect(timeline.locator(".transcript-scene-header")).toHaveCount(0);
    await expect(timeline.locator(".transcript-avatar")).toHaveCount(0);
    await expect(meeting.locator(".recording-context")).toHaveCount(6);
    await expect(meeting.locator(".recording-context").filter({ hasText: "微信输入法" }).first().locator("img")).toBeVisible();
    await expect(meeting.locator(".recording-context").filter({ hasText: "LongApplicationName" }).first().locator("svg")).toBeVisible();
    const system = timeline.locator('.transcript-event[data-audio-source="system"]');
    await expect(system.locator(".transcript-speaker-name")).toHaveText(unknown);
    await expect(system.getByTestId("audio-source-label")).toHaveText(language === "zh-CN" ? "系统声音" : "System audio");
    await expect(system.locator(".recording-context")).toHaveCount(0);
    await expect(timeline.getByRole("article").filter({ hasText: "旧录音" }).locator("time"))
      .toHaveText(language === "zh-CN" ? "时间未知" : "Unknown time");
    expect(await meeting.evaluateAll(nodes => nodes.every(node => {
      const dot = getComputedStyle(node, "::before");
      const speaker = getComputedStyle(node.querySelector(".transcript-speaker-name")!);
      return node.getAttribute("data-voice-identity") === "me" ? dot.backgroundColor === speaker.color : dot.backgroundColor !== speaker.color;
    }))).toBe(true);

    for (const width of role === "main" ? [1000, 720, 380] : [420, 380]) {
      await page.setViewportSize({ width, height: 900 });
      await expect.poll(() => timeline.locator(".transcript-turn").evaluateAll(elements => elements.every(element => {
        const rect = element.getBoundingClientRect();
        return rect.left >= 16 && rect.right <= innerWidth - 16 && element.scrollWidth <= element.clientWidth + 1;
      }))).toBe(true);
      await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
      // The same rail continues through all speakers and recording sources.
      await expect.poll(() => timeline.locator(".transcript-event").evaluateAll(rows => rows.every((row, index) => {
        const rail = getComputedStyle(row, "::before");
        const next = rows[index + 1];
        if (!next) return rail.content === "none";
        const nextDot = getComputedStyle(next.querySelector(".transcript-turn")!, "::before");
        const bounds = row.getBoundingClientRect(), nextBounds = next.getBoundingClientRect();
        return rail.content !== "none" && Math.abs(bounds.bottom - parseFloat(rail.bottom) - nextBounds.top - parseFloat(nextDot.top) - parseFloat(nextDot.height) / 2) < 1;
      }))).toBe(true);
      await page.addStyleTag({ content: `html { background: ${theme === "dark" ? "#1c1c1e" : "#f5f5f7"}; }` });
      await page.evaluate(() => document.fonts.ready);
      const screenshot = testInfo.outputPath(`transcript-${role}-${language}-${theme}-${width}.png`);
      await page.screenshot({ path: screenshot, animations: "disabled" });
      await testInfo.attach(`transcript-${width}`, { path: screenshot, contentType: "image/png" });
    }
    if (role === "main") {
      await page.setViewportSize({ width: 1000, height: 900 });
      await meeting.getByRole("button", { name: /微信输入法/ }).first().click();
      await expect(page.getByRole("combobox", { name: /Filter by app|按应用筛选/ })).toHaveValue("wetype");
      await expect(timeline.getByRole("article")).toHaveCount(3);
    }
    await app.close();
  });
}
