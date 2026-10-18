import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import type { AppLanguage, DesktopSnapshot } from "@daybee/shared";
import { expect, test } from "./fixtures";

for (const language of ["zh-CN", "en-US"] as const) for (const theme of ["light", "dark"] as const) {
  test(`shows compact transcripts in ${language} ${theme} at regular and minimum sizes`, async ({ launchApp }, testInfo) => {
    const { app, page } = await launchApp();
    await page.emulateMedia({ colorScheme: theme });
    const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
    const directory = snapshot.settings.recording.outputDir;
    mkdirSync(directory, { recursive: true });
    for (const [name, at, text] of [
      ["first", "2026-10-09T10:00:00Z", "今天讨论录音"],
      ["second", "2026-10-09T10:00:10Z", "质量和转写结果。\n\nWe agreed to review the complete transcript this afternoon."]
    ]) {
      writeFileSync(join(directory, `${name}.json`), JSON.stringify({
        audio_path: join(directory, `${name}.flac`), segment_start_time: at, status: "ok", text
      }));
    }
    await app.refreshHistory();
    await page.evaluate(async ({ language, theme }: { language: AppLanguage; theme: "light" | "dark" }) => {
      const current = await window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap");
      await window.mygo!.call("DesktopService.SaveSettings", {
        ...current.settings, desktop: { ...current.settings.desktop, language, theme }
      });
    }, { language, theme });
    const timeline = page.getByRole("region", { name: /Transcription timeline|转写时间线/ });
    await expect(timeline.getByRole("article")).toHaveCount(2);
    await expect(timeline).toContainText("今天讨论录音质量和转写结果。");
    await expect(page.locator("html")).toHaveClass(theme === "dark" ? /dark/ : /^$/);
    await expect(page.getByRole("tablist")).toBeVisible();
    await expect(page.getByRole("region", { name: /Recent recordings|最近录音/ })).toHaveCount(0);
    await page.addStyleTag({ content: `html { background: ${theme === "dark" ? "#1c1c1e" : "#f5f5f7"}; }` });
    for (const [size, width, height] of [["regular", 420, 620], ["minimum", 380, 480]] as const) {
      await page.setViewportSize({ width, height });
      const times = await timeline.locator("time").evaluateAll(elements => elements.map(element => element.getAttribute("datetime")));
      expect(times).toEqual(["2026-10-09T10:00:00.000Z", "2026-10-09T10:00:10.000Z", "2026-10-09T10:00:10.000Z"]);
      expect(await timeline.locator(".transcript-entry").evaluateAll(elements => elements.every(element => {
        const rect = element.getBoundingClientRect();
        return rect.left >= 16 && rect.right <= innerWidth - 16;
      }))).toBe(true);
      await page.evaluate(() => document.fonts.ready);
      const screenshot = testInfo.outputPath(`compact-${language}-${theme}-${size}.png`);
      await page.screenshot({ path: screenshot, animations: "disabled" });
      await testInfo.attach(`compact-${language}-${theme}-${size}`, { path: screenshot, contentType: "image/png" });
    }
    await app.close();
  });
}
