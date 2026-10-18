import type { DesktopSnapshot, RecordingHistoryItem } from "@daybee/shared";
import { expect, test } from "./fixtures";

const at = (second: number) => new Date(Date.UTC(2026, 9, 12) + second * 1000).toISOString();

for (const [role, language] of [["compact", "zh-CN"], ["compact", "en-US"]] as const) {
  test(`retains complete utterances without promoting unconfirmed speech in ${language}`, async ({ launchApp }) => {
    const { app, page } = await launchApp({ voiceprintSetup: "ready" });
    await page.goto("/");
    await page.setViewportSize({ width: 420, height: 900 });
    const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    snapshot.settings.desktop.language = language;
    const item: RecordingHistoryItem = {
      id: "utterance-contract", sessionId: "session", audioSource: "microphone", audioPath: "/synthetic.wav",
      folderPath: "/synthetic", inputDevice: null, status: "ok", startedAt: at(0),
      textPreview: "One continuous\ncomplete sentence\nretains its ending.\nA full new utterance.\nAnother unconfirmed utterance.",
      capturedRanges: [{ startedAt: at(0), endedAt: at(8) }, { startedAt: at(20), endedAt: at(23) }, { startedAt: at(24), endedAt: at(26) }],
      transcriptChunks: [
        { text: "One continuous", startedAt: at(0), endedAt: at(4), voiceIdentity: "me" },
        { text: "complete sentence", startedAt: at(4), endedAt: at(6) },
        { text: "retains its ending.", startedAt: at(6), endedAt: at(8), voiceIdentity: "me" },
        { text: "A full new utterance.", startedAt: at(20), endedAt: at(23), voiceMatch: "partial" },
        { text: "Another unconfirmed utterance.", startedAt: at(24), endedAt: at(26) }
      ]
    };
    snapshot.history = [item];
    await app.setFixture(snapshot);
    const rows = page.locator(".transcript-turn");
    await expect(rows).toHaveCount(3);
    const historical = rows.filter({ hasText: "One continuous" });
    await expect(historical).toContainText("complete sentence");
    await expect(historical).toContainText("retains its ending.");
    await expect(historical).toHaveAttribute("data-voice-identity", "partial");
    await expect(historical.locator(".transcript-speaker-name")).toHaveText(language === "zh-CN" ? "部分匹配本人" : "Partial voiceprint match");
    expect(await historical.locator("header time").evaluateAll(times => times.map(time => time.getAttribute("datetime"))))
      .toEqual([at(0), at(8)]);
    await expect(rows.filter({ hasText: "A full new utterance." })).toHaveAttribute("data-voice-identity", "partial");
    await expect(rows.filter({ hasText: "Another unconfirmed utterance." })).toHaveAttribute("data-voice-identity", "unknown");
    await expect(rows.filter({ hasText: "Another unconfirmed utterance." }).locator(".transcript-speaker-name"))
      .toHaveText(language === "zh-CN" ? "身份未确认" : "Identity unconfirmed");
    await expect(page.locator('.transcript-turn[data-voice-identity="me"]')).toHaveCount(0);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: `e2e/results/voiceprint/utterance-continuity-${role}.png`, animations: "disabled" });
    await app.close();
  });
}
