import { join } from "node:path";
import { mkdirSync, writeFileSync } from "node:fs";
import type { Locator, Page } from "@playwright/test";
import { DEFAULT_SETTINGS, type AppSettings, type DesktopSnapshot } from "@daybee/shared";
import { expect, test } from "./fixtures";

test("keeps recording controls compact and moves settings to sidebar", async ({ launchApp }) => {
  const { app, page } = await launchApp();

  await expect(page.getByRole("button", { name: /Start recording|开始录音|Stop|停止/ })).toBeVisible();
  await expect(page.getByRole("tab")).toHaveCount(0);
  await expect(page.getByText(/Live transcript|实时转写/, { exact: true })).toHaveCount(0);
  await page.locator(".tray-panel").evaluate(async panel => {
    await Promise.all(panel.getAnimations().map(animation => animation.finished));
  });
  const margins = await page.locator(".panel-section").first().evaluate(section => {
    const content = section.firstElementChild!.getBoundingClientRect();
    const drag = document.querySelector(".drag-region")!.getBoundingClientRect();
    const controls = document.querySelector(".windows-window-controls")?.getBoundingClientRect();
    return { top: content.top, left: content.left, right: innerWidth - content.right, dragBottom: drag.bottom,
      windows: window.mygo!.platform === "win32", controlsBottom: controls?.bottom ?? 0 };
  });
  expect(margins.left).toBe(margins.right);
  if (margins.windows) {
    expect(margins.controlsBottom).toBeGreaterThan(0);
    expect(margins.top).toBeGreaterThanOrEqual(margins.controlsBottom);
  } else {
    expect(margins.top).toBe(margins.left);
  }
  expect(margins.dragBottom).toBeLessThanOrEqual(margins.top);
  await page.getByRole("button", { name: /Open main window|打开主窗口/ }).click();
  await page.goto("/?window=main");
  await page.setViewportSize({ width: 1000, height: 760 });
  const navigation = page.getByRole("navigation", { name: /Workspace navigation|主窗口导航/ });
  for (const label of [/Home|首页/, /History|历史/, /General|通用/, /Input|输入/, /Transcribe|转写/]) {
    await expect(navigation.getByRole("button", { name: label })).toBeVisible();
  }
  await expect(navigation.getByRole("group", { name: /Workspace|工作空间/, exact: true }).getByRole("button")).toHaveCount(2);
  await expect(navigation.getByRole("group", { name: /Settings|设置/, exact: true }).getByRole("button")).toHaveCount(3);
  // Voiceprint enrollment has its own recording action in the Home content.
  await expect(page.locator(".workspace-toolbar").getByRole("button", { name: /Start recording|开始录音/ })).toHaveCount(0);

  const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
  expect(snapshot.engineReady).toBe(true);
  expect(snapshot.permission.state).toBe("authorized");

  await app.close();
});

test("can start and stop a recording session", async ({ launchApp }) => {
  const { app, page } = await launchApp();

  await ensureIdle(page);
  await page.getByRole("button", { name: /Start recording|开始录音/ }).click();
  await expect(page.getByRole("button", { name: /Stop|停止/ })).toBeVisible();
  await expect.poll(async () => (await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"))).status.recording).toBe(true);

  await page.getByRole("button", { name: /Stop|停止/ }).click();
  await expect(page.getByRole("button", { name: /Start recording|开始录音/ })).toBeVisible();
  await expect.poll(async () => (await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"))).status.recording).toBe(false);

  await app.close();
});

async function transcriptRows(region: Locator) {
  return region.locator(".transcription-timeline").evaluate(async list => {
    const viewport = list.closest("[data-radix-scroll-area-viewport]")!;
    const original = viewport.scrollTop;
    const rows = new Map<number, { time: string; text: string }>();
    const frame = () => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    viewport.scrollTop = 0;
    await frame();
    // Virtual rows outside the viewport are intentionally unmounted. Read the
    // entire small fixture by scrolling, rather than treating the DOM as history.
    for (let n = 0; n < 100; n++) {
      for (const element of Array.from(list.querySelectorAll<HTMLElement>(".transcript-event"))) {
        rows.set(Number(element.dataset.index), {
          time: Array.from(element.querySelectorAll("time")).at(-1)?.dateTime ?? "",
          text: Array.from(element.querySelectorAll(".transcript-text")).map(text => text.textContent).join("")
        });
      }
      if (list.querySelector('[data-last="true"]')) break;
      viewport.scrollTop += viewport.clientHeight;
      await frame();
    }
    viewport.scrollTop = original;
    await frame();
    let time = "";
    return [...rows].sort(([a], [b]) => a - b).map(([, row]) => {
      if (row.time) time = row.time;
      return { time, text: row.text };
    });
  });
}

test("shares complete merged transcripts with Home and keeps file history in the workspace", async ({ launchApp }) => {
  const { app, page } = await launchApp();
  const timeline = page.getByRole("region", { name: /Transcription timeline|转写时间线/ });
  await expect(timeline.getByRole("heading", { name: /No activity yet|还没有活动记录/ })).toBeVisible();
  await expect(timeline.getByRole("button", { name: /Recording controls|录音控制/ })).toHaveCount(0);
  const snapshot = await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"));
  const texts = [
    "较早的完整转写。".repeat(50), "今天讨论录音", "质量和转写结果。",
    `${"最近的会议内容。".repeat(55)}\n\n${"LongUnbrokenWord".repeat(25)}全文结束🙂。`
  ];
  const folders = ["2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09"].map((day, index) => {
    const folder = join(snapshot.settings.recording.outputDir, day);
    mkdirSync(folder, { recursive: true });
    writeFileSync(join(folder, "completed.json"), JSON.stringify({
      audio_path: join(folder, "completed.flac"), segment_start_time: `${day}T12:00:00Z`, status: "ok", text: texts[index]
    }));
    return folder;
  });
  const pendingPath = join(folders[3], "pending.json");
  const pending = { audio_path: join(folders[3], "pending.flac"), segment_start_time: "2026-10-09T13:00:00Z", status: "audio_only" };
  writeFileSync(pendingPath, JSON.stringify(pending));
  await app.refreshHistory();

  await expect(timeline.locator(".transcript-text").filter({ hasText: "今天讨论录音" })).toHaveText("今天讨论录音");
  await expect(timeline.locator(".transcript-text").filter({ hasText: "质量和转写结果。" })).toHaveText("质量和转写结果。");
  await expect(timeline).toContainText("全文结束🙂。");
  const initialRows = await transcriptRows(timeline);
  expect(initialRows.length).toBeGreaterThan(4);
  const chronologicalText = [...initialRows].sort((a, b) => Date.parse(a.time) - Date.parse(b.time)).map(row => row.text).join("");
  expect(chronologicalText.replace(/\s/gu, "")).toBe(texts.join("").replace(/\s/gu, ""));
  expect(initialRows[0].time).toBe("2026-10-09T12:00:00.000Z");
  expect(initialRows.at(-1)!.time).toBe("2026-10-06T12:00:00.000Z");
  await expect(timeline.getByRole("button", { name: /Open this day's recording folder|打开当天录音文件夹/ })).toHaveCount(0);
  await expect(timeline).not.toContainText("completed.flac");

  // Completing an audio segment updates the compact timeline without navigation or reload.
  const completedText = `${"刚刚完成的新转写。".repeat(45)}自动更新结束标记。`;
  writeFileSync(pendingPath, JSON.stringify({ ...pending, status: "ok", text: completedText }));
  await app.refreshHistory();
  await expect(timeline).toContainText("自动更新结束标记。");
  const updatedRows = await transcriptRows(timeline);
  expect(updatedRows[0].time).toBe("2026-10-09T13:00:00.000Z");
  await page.setViewportSize({ width: 380, height: 480 });
  expect(await timeline.locator(".transcript-paragraph").evaluateAll(elements => elements.every(element =>
    element.clientHeight >= element.scrollHeight && element.clientWidth >= element.scrollWidth &&
    getComputedStyle(element).overflowWrap === "anywhere"
  ))).toBe(true);
  await timeline.getByRole("article").last().scrollIntoViewIfNeeded();
  await expect(timeline.getByRole("article").last()).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);

  await openWorkspace(page);
  const home = page.getByRole("region", { name: /Home|首页/, exact: true });
  await expect.poll(() => transcriptRows(home)).toEqual(updatedRows);
  await page.getByRole("navigation").getByRole("button", { name: /History|历史/, exact: true }).click();
  const history = page.getByRole("region", { name: /History|历史/, exact: true });
  await expect(history.getByRole("button", { name: /Open this day's recording folder|打开当天录音文件夹/ })).toHaveCount(4);
  await expect(history.getByText(folders[0], { exact: true })).toBeVisible();
  await app.close();
});

test("keeps speech activity on when realtime transcription is disabled", async ({ launchApp }) => {
  const { app, page } = await launchApp();

  await openWorkspace(page);
  await page.getByRole("navigation").getByRole("button", { name: /Transcribe|转写/ }).click();
  const realtimeAsrSwitch = page.getByTestId("realtime-asr-field").locator('[role="switch"]');
  await realtimeAsrSwitch.click();

  await page.goto("/");
  await ensureIdle(page);
  await page.getByRole("button", { name: /Start recording|开始录音/ }).click();
  await expect.poll(async () => (await page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"))).status.inSpeech).toBe(true);

  await page.getByRole("button", { name: /Stop|停止/ }).click();
  await app.close();
});

test("persists general settings across relaunch", async ({ launchApp }) => {
  let context = await launchApp();
  let switchLocator = await openLaunchAtLoginSwitch(context.page);
  const initialState = await switchLocator.getAttribute("aria-checked");
  const nextState = initialState === "true" ? "false" : "true";

  await switchLocator.click();
  await expect(switchLocator).toHaveAttribute("aria-checked", nextState);
  await context.app.close();

  context = await launchApp();
  switchLocator = await openLaunchAtLoginSwitch(context.page);
  await expect(switchLocator).toHaveAttribute("aria-checked", nextState);
  await context.app.close();
});

test("persists every setting field across relaunch", async ({ launchApp, storeDir }) => {
  let context = await launchApp();
  const expectedSettings = buildCustomSettings(storeDir);

  await context.page.evaluate(async (settings: AppSettings) => {
    await window.mygo!.call("DesktopService.SaveSettings", settings);
  }, expectedSettings);

  await expect
    .poll(async () => (await context.page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"))).settings)
    .toEqual(expectedSettings);

  await context.app.close();

  context = await launchApp();
  await expect
    .poll(async () => (await context.page.evaluate(() => window.mygo!.call<DesktopSnapshot>("DesktopService.Bootstrap"))).settings)
    .toEqual(expectedSettings);
  await context.app.close();
});

async function openLaunchAtLoginSwitch(page: Page) {
  await openWorkspace(page);
  await page.getByRole("navigation").getByRole("button", { name: /General|通用/ }).click();
  const switchLocator = page.getByTestId("launch-at-login-field").locator('[role="switch"]');
  await expect(switchLocator).toBeVisible();
  return switchLocator;
}

test("supports keyboard navigation and sidebar collapse", async ({ launchApp }) => {
  const { app, page } = await launchApp();
  await openWorkspace(page);
  const nav = page.getByRole("navigation");
  await nav.getByRole("button", { name: /History|历史/ }).focus();
  await page.keyboard.press("ArrowDown");
  await expect(nav.getByRole("button", { name: /General|通用/ })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("heading", { name: /General|通用/, exact: true })).toBeVisible();
  await page.keyboard.press("ArrowUp");
  await expect(nav.getByRole("button", { name: /History|历史/ })).toBeFocused();
  await page.keyboard.press("End");
  await expect(nav.getByRole("button", { name: /Transcribe|转写/ })).toBeFocused();
  await page.keyboard.press("ArrowDown");
  await expect(nav.getByRole("button", { name: /Home|首页/ })).toBeFocused();
  await page.keyboard.press("Home");
  await expect(nav.getByRole("button", { name: /Home|首页/ })).toBeFocused();
  await page.getByRole("button", { name: /Toggle sidebar|切换侧栏/ }).click();
  await expect(nav).not.toBeVisible();
  await page.getByRole("button", { name: /Toggle sidebar|切换侧栏/ }).click();
  await expect(nav).toBeVisible();
  await app.close();
});

test("uses a drawer for a narrow main window", async ({ launchApp }) => {
  const { app, page } = await launchApp();
  await openWorkspace(page);
  await page.setViewportSize({ width: 720, height: 540 });
  await page.getByRole("button", { name: /Toggle sidebar|切换侧栏/ }).click();
  const drawer = page.getByRole("dialog", { name: "Sidebar" });
  await expect(drawer).toBeVisible();
  await drawer.getByRole("button", { name: /Input|输入/, exact: true }).click();
  await expect(drawer).not.toBeVisible();
  await expect(page.getByRole("heading", { name: /Input|输入/, exact: true })).toBeVisible();
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await app.close();
});

async function openWorkspace(page: Page) {
  await page.goto("/?window=main");
  await page.setViewportSize({ width: 1000, height: 760 });
}

async function ensureIdle(page: Page) {
  const stopButton = page.getByRole("button", { name: /Stop|停止/ });
  if (await stopButton.isVisible()) {
    await stopButton.click();
  }
  await expect(page.getByRole("button", { name: /Start recording|开始录音/ })).toBeVisible();
}

function buildCustomSettings(storeDir: string): AppSettings {
  return {
    desktop: {
      ...DEFAULT_SETTINGS.desktop,
      language: "en-US",
      launchAtLogin: false,
      startRecordingOnLaunch: false,
      theme: "light"
    },
    recording: {
      ...DEFAULT_SETTINGS.recording,
      audioFormat: "wav",
      asrLanguage: "ja",
      echoCancellation: process.platform === "darwin",
      autoSwitchConfirmations: 3,
      autoSwitchDevice: false,
      device: "usb-mic",
      disableAsr: true,
      excludeDeviceKeywords: "bluetooth,continuity",
      outputDir: join(storeDir, "recordings-out"),
      segmentMinutes: 90
    }
  };
}
