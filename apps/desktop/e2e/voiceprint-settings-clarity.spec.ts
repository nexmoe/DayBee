import type { DesktopSnapshot } from "@daybee/shared";
import { expect, test } from "./fixtures";

for (const language of ["zh-CN", "en-US"] as const) {
  for (const width of [1000, 480]) {
    test(`keeps voiceprint setup clear and secondary tools folded in ${language} at ${width}px`, async ({ launchApp }) => {
      const { app, page } = await launchApp({ voiceprintSetup: "ready" });
      await page.emulateMedia({ colorScheme: "light" });
      await page.goto("/?window=main");
      await page.setViewportSize({ width, height: 900 });
      await page.addStyleTag({ content: "html { background: #f5f5f7; }" });
      const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
      snapshot.settings.desktop.language = language;
      snapshot.settings.recording.voiceprintAdaptation = false;
      snapshot.settings.recording.disableAsr = false;
      snapshot.status.asrEnabled = true;
      snapshot.status.recording = false;
      snapshot.status.recovering = false;
      snapshot.status.voiceprintReference = "unavailable";
      snapshot.status.voiceprintContextMismatch = false;
      snapshot.voiceprint.enrolled = true;
      snapshot.voiceprint.phase = "ready";
      snapshot.voiceprint.adaptationCount = 0;
      snapshot.asrModels.busy = false;
      snapshot.history = [{
        audioSource: "microphone", audioPath: "/recordings/meeting.wav", folderPath: "/recordings", id: "meeting",
        inputDevice: null, startedAt: "2026-10-12T14:13:15.974318+08:00", status: "ok", sessionId: "meeting",
        textPreview: "这是一段很长的会议记录，用来确认诊断列表会简短显示，而不会用内部时间戳和整段转写撑开窗口。".repeat(8),
        transcriptChunks: []
      }];
      await app.setFixture(snapshot);
      if (width < 768) await page.getByRole("button", { name: /切换侧栏|Toggle sidebar/i }).click();
      await page.getByRole("navigation").getByRole("button", { name: /Transcribe|转写/, exact: true }).click();
      const manage = page.getByTestId("voiceprint-manage-field");
      await expect(manage).toContainText(language === "zh-CN" ? "已录入" : "voiceprint is ready");
      await expect(manage).toContainText(language === "zh-CN" ? "标记为「我」" : "marked as Me");
      await expect(manage).not.toContainText(/\d\s*\/\s*32|学习我的声音|Learn my voice/);
      await expect(page.getByTestId("voiceprint-runtime-hints")).toHaveCount(0);
      await expect(page.getByTestId("voiceprint-advanced")).not.toHaveAttribute("open");
      await expect(page.getByTestId("voiceprint-adaptation-field")).not.toBeVisible();
      await expect(page.getByTestId("voiceprint-diagnostics")).not.toHaveAttribute("open");
      await expect(page.getByTestId("voiceprint-export-field")).not.toBeVisible();
      await manage.screenshot({ path: `e2e/results/voiceprint/clear-default-${language}-${width}.png`, animations: "disabled" });

      await page.getByTestId("voiceprint-advanced-toggle").click();
      const adaptation = page.getByTestId("voiceprint-adaptation-field");
      await expect(adaptation).toBeVisible();
      await expect(adaptation).toContainText(language === "zh-CN" ? "保持关闭也能标记" : "works with this off");
      await expect(adaptation.getByRole("switch")).not.toBeChecked();
      await expect(page.getByTestId("voiceprint-adaptation-clear")).toBeDisabled();
      await manage.screenshot({ path: `e2e/results/voiceprint/clear-advanced-${language}-${width}.png`, animations: "disabled" });

      snapshot.settings.recording.voiceprintAdaptation = true;
      snapshot.status.recording = true;
      snapshot.status.voiceprintReference = "available";
      snapshot.voiceprint.adaptationCount = 32;
      snapshot.voiceprint.adaptation = { learnedClips: 47, assistedClips: 8 };
      await app.setFixture(snapshot);
      const progress = page.getByTestId("voiceprint-adaptation-progress");
      await expect(progress).toBeVisible();
      await expect(page.getByTestId("voiceprint-adaptation-learned")).toContainText("47");
      await expect(page.getByTestId("voiceprint-adaptation-assisted")).toContainText("8");
      await expect(progress).not.toContainText("%");
      await expect(page.getByTestId("voiceprint-runtime-hints")).toHaveCount(0);
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
      await manage.screenshot({ path: `e2e/results/voiceprint/learning-${language}-${width}.png`, animations: "disabled" });

      snapshot.voiceprint.adaptation = { learnedClips: 48, assistedClips: 9 };
      await app.setFixture(snapshot);
      await expect(page.getByTestId("voiceprint-adaptation-learned")).toContainText("48");
      await expect(page.getByTestId("voiceprint-adaptation-assisted")).toContainText("9");
      snapshot.settings.desktop.theme = "dark";
      await page.emulateMedia({ colorScheme: "dark" });
      await page.addStyleTag({ content: "html { background: #1c1c1e; }" });
      await app.setFixture(snapshot);
      await expect(page.locator("html")).toHaveClass(/dark/);
      await expect(manage).toHaveCSS("color", "rgba(255, 255, 255, 0.9)");
      await manage.screenshot({ path: `e2e/results/voiceprint/learning-dark-${language}-${width}.png`, animations: "disabled" });

      // Older profiles saved window samples, so do not present them as learned clips.
      delete snapshot.voiceprint.adaptation;
      await app.setFixture(snapshot);
      await expect(page.getByTestId("voiceprint-adaptation-learned")).toContainText(language === "zh-CN" ? "已保留的声音样本" : "Saved voice samples");
      await expect(page.getByTestId("voiceprint-adaptation-learned")).toContainText("32");

      snapshot.settings.recording.voiceprintAdaptation = false;
      snapshot.status.recording = false;
      await app.setFixture(snapshot);
      await expect(progress).not.toBeVisible();

      await page.getByTestId("voiceprint-diagnostics-toggle").click();
      const exported = page.getByTestId("voiceprint-export-field");
      await expect(exported).toBeVisible();
      const select = exported.getByRole("combobox");
      await expect(select.locator("option")).toHaveCount(1);
      const label = await select.locator("option").innerText();
      expect(label).not.toContain("T14:13:15");
      expect(label.length).toBeLessThan(100);
      await expect(page.getByTestId("voiceprint-export-button")).toBeEnabled();
      expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
      await page.getByTestId("voiceprint-diagnostics").screenshot({ path: `e2e/results/voiceprint/clear-diagnostics-${language}-${width}.png`, animations: "disabled" });

      snapshot.status.recording = true;
      await app.setFixture(snapshot);
      await expect(page.getByTestId("voiceprint-export-button")).toBeDisabled();
      await expect(exported).toContainText(language === "zh-CN" ? "请先结束录音和转写" : "Finish recording and transcription");
    });
  }
}
